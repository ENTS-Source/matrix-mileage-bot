package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

type Bot struct {
	client     *mautrix.Client
	store      *Store
	cfg        *Config
	tiers      []RateTier
	location   *time.Location
	authorized map[id.UserID]struct{}
	rooms      map[id.RoomID]struct{}
}

func newBot(client *mautrix.Client, store *Store, cfg *Config, tiers []RateTier) (*Bot, error) {
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return nil, fmt.Errorf("load timezone %q: %w", cfg.Timezone, err)
	}
	b := &Bot{
		client:     client,
		store:      store,
		cfg:        cfg,
		tiers:      tiers,
		location:   loc,
		authorized: make(map[id.UserID]struct{}, len(cfg.AuthorizedUsers)),
		rooms:      make(map[id.RoomID]struct{}, len(cfg.Matrix.AllowedRoomIDs)),
	}
	for _, userID := range cfg.AuthorizedUsers {
		b.authorized[id.UserID(userID)] = struct{}{}
	}
	for _, roomID := range cfg.Matrix.AllowedRoomIDs {
		b.rooms[id.RoomID(roomID)] = struct{}{}
	}
	return b, nil
}

func (b *Bot) handleMessage(ctx context.Context, evt *event.Event) {
	if evt.Sender == b.client.UserID || !b.roomAllowed(evt.RoomID) {
		return
	}
	msg := evt.Content.AsMessage()
	if msg.MsgType != event.MsgText && msg.MsgType != event.MsgNotice {
		return
	}
	body := strings.TrimSpace(msg.Body)
	if !strings.HasPrefix(body, "!mileage") {
		return
	}

	fields := strings.Fields(body)
	if len(fields) == 0 || fields[0] != "!mileage" {
		return
	}

	var err error
	switch {
	case len(fields) == 1:
		b.notice(ctx, evt.RoomID, usageText())
		return
	case fields[1] == "start":
		err = b.commandStart(ctx, evt, fields)
	case fields[1] == "end":
		err = b.commandEnd(ctx, evt, fields)
	case fields[1] == "report":
		err = b.commandReport(ctx, evt, fields)
	case fields[1] == "reset":
		err = b.commandReset(ctx, evt, fields)
	default:
		err = b.commandStandalone(ctx, evt, fields)
	}
	if err != nil {
		slog.Error("command failed", "sender", evt.Sender, "room", evt.RoomID, "event", evt.ID, "error", err)
		b.notice(ctx, evt.RoomID, "Error: "+err.Error())
	}
}

func (b *Bot) commandStart(ctx context.Context, evt *event.Event, fields []string) error {
	if len(fields) != 3 {
		return fmt.Errorf("usage: !mileage start <odometer>")
	}
	odo, err := parseKilometersMilli(fields[2])
	if err != nil {
		return fmt.Errorf("invalid odometer: %w", err)
	}
	now := b.eventTime(evt)
	if err := b.store.StartOdometer(string(evt.Sender), now.Format("Jan 02, 2006"), b.cfg.Purpose, odo, now); err != nil {
		if errors.Is(err, ErrAlreadyActive) {
			return fmt.Errorf("you already have an active odometer record; end it before starting another")
		}
		return err
	}
	b.notice(ctx, evt.RoomID, fmt.Sprintf("Mileage started at %s km.", formatKM(odo)))
	return nil
}

func (b *Bot) commandEnd(ctx context.Context, evt *event.Event, fields []string) error {
	if len(fields) != 3 {
		return fmt.Errorf("usage: !mileage end <odometer>")
	}
	odo, err := parseKilometersMilli(fields[2])
	if err != nil {
		return fmt.Errorf("invalid odometer: %w", err)
	}
	now := b.eventTime(evt)
	rec, err := b.store.EndOdometer(string(evt.Sender), now.Format("Jan 02, 2006"), odo, now)
	if errors.Is(err, ErrNoActive) {
		return fmt.Errorf("you do not have an active odometer record")
	}
	if err != nil {
		return err
	}
	b.notice(ctx, evt.RoomID, fmt.Sprintf("Recorded %s km for %s.", formatKM(rec.KilometersMilli), rec.Purpose))
	return nil
}

func (b *Bot) commandStandalone(ctx context.Context, evt *event.Event, fields []string) error {
	if len(fields) != 2 {
		return fmt.Errorf("usage: !mileage <km>")
	}
	km, err := parseKilometersMilli(fields[1])
	if err != nil {
		return fmt.Errorf("invalid kilometers: %w", err)
	}
	now := b.eventTime(evt)
	if _, err := b.store.AddStandalone(string(evt.Sender), now.Format("Jan 02, 2006"), b.cfg.Purpose, km, now); err != nil {
		return err
	}
	b.notice(ctx, evt.RoomID, fmt.Sprintf("Recorded %s km for %s.", formatKM(km), b.cfg.Purpose))
	return nil
}

func (b *Bot) commandReport(ctx context.Context, evt *event.Event, fields []string) error {
	if len(fields) != 2 {
		return fmt.Errorf("usage: !mileage report")
	}
	if !b.isAuthorized(evt.Sender) {
		return fmt.Errorf("you are not authorized to generate mileage reports")
	}

	byUser, err := b.store.RecordsByUser()
	if err != nil {
		return err
	}
	if len(byUser) == 0 {
		b.notice(ctx, evt.RoomID, "There are no mileage records to report.")
		return nil
	}

	userIDs := make([]string, 0, len(byUser))
	for userID := range byUser {
		userIDs = append(userIDs, userID)
	}
	sort.Strings(userIDs)

	for _, userID := range userIDs {
		displayName := localpart(userID)
		profile, profileErr := b.client.GetDisplayName(ctx, id.UserID(userID))
		if profileErr == nil && strings.TrimSpace(profile.DisplayName) != "" {
			displayName = profile.DisplayName
		}

		report, err := generatePDFReport(displayName, userID, localpart(userID), b.cfg.Reimbursement.Currency, byUser[userID], b.tiers, time.Now().In(b.location))
		if err != nil {
			return fmt.Errorf("generate report for %s: %w", userID, err)
		}
		upload, err := b.client.UploadMedia(ctx, mautrix.ReqUploadMedia{
			ContentBytes: report.PDF,
			ContentType:  "application/pdf",
			FileName:     report.FileName,
		})
		if err != nil {
			return fmt.Errorf("upload report for %s: %w", userID, err)
		}
		content := map[string]any{
			"msgtype":  "m.file",
			"body":     report.FileName,
			"filename": report.FileName,
			"url":      upload.ContentURI.String(),
			"info": map[string]any{
				"mimetype": "application/pdf",
				"size":     len(report.PDF),
			},
		}
		if _, err := b.client.SendMessageEvent(ctx, evt.RoomID, event.EventMessage, content); err != nil {
			return fmt.Errorf("send report for %s: %w", userID, err)
		}
	}
	return nil
}

func (b *Bot) commandReset(ctx context.Context, evt *event.Event, fields []string) error {
	if len(fields) != 2 {
		return fmt.Errorf("usage: !mileage reset")
	}
	if !b.isAuthorized(evt.Sender) {
		return fmt.Errorf("you are not authorized to reset mileage records")
	}
	records, active, err := b.store.ResetMileage()
	if err != nil {
		return err
	}
	b.notice(ctx, evt.RoomID, fmt.Sprintf("Mileage reset complete: deleted %d completed record(s) and %d active odometer record(s).", records, active))
	return nil
}

func (b *Bot) notice(ctx context.Context, roomID id.RoomID, text string) {
	if _, err := b.client.SendNotice(ctx, roomID, text); err != nil {
		slog.Error("failed to send Matrix notice", "room", roomID, "error", err)
	}
}

func (b *Bot) roomAllowed(roomID id.RoomID) bool {
	if len(b.rooms) == 0 {
		return true
	}
	_, ok := b.rooms[roomID]
	return ok
}

func (b *Bot) isAuthorized(userID id.UserID) bool {
	_, ok := b.authorized[userID]
	return ok
}

func (b *Bot) eventTime(evt *event.Event) time.Time {
	if evt.Timestamp > 0 {
		return time.UnixMilli(evt.Timestamp).In(b.location)
	}
	return time.Now().In(b.location)
}

func localpart(userID string) string {
	s := strings.TrimPrefix(userID, "@")
	if i := strings.IndexByte(s, ':'); i >= 0 {
		return s[:i]
	}
	return s
}

func usageText() string {
	return strings.Join([]string{
		"Mileage commands:",
		"!mileage start <odometer>",
		"!mileage end <odometer>",
		"!mileage <km>",
		"!mileage report (authorized users only)",
		"!mileage reset (authorized users only)",
	}, "\n")
}
