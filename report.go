package main

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
)

type GeneratedReport struct {
	FileName     string
	PDF          []byte
	TotalCents   int64
	TotalMilliKM int64
}

func generatePDFReport(displayName, userID, localpart, currency string, records []Record, tiers []RateTier, generated time.Time) (GeneratedReport, error) {
	var totalKM int64
	for _, rec := range records {
		totalKM += rec.KilometersMilli
	}
	totalCents, breakdown := calculateReimbursement(totalKM, tiers)

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(18, 16, 18)
	pdf.SetAutoPageBreak(true, 18)
	translate := pdf.UnicodeTranslatorFromDescriptor("")
	t := func(s string) string { return translate(s) }

	writeHeader := func() {
		pdf.SetFont("Arial", "B", 16)
		pdf.CellFormat(0, 9, t("Mileage Reimbursement Report"), "", 1, "L", false, 0, "")
		pdf.SetFont("Arial", "", 10)
		pdf.CellFormat(0, 6, t("Name: "+displayName), "", 1, "L", false, 0, "")
		pdf.CellFormat(0, 6, t("Matrix user ID: "+userID), "", 1, "L", false, 0, "")
		pdf.CellFormat(0, 6, t("Generated: "+generated.Format("Jan 02, 2006")), "", 1, "L", false, 0, "")
		pdf.Ln(3)
	}

	writeTableHeader := func() {
		pdf.SetFont("Arial", "B", 9)
		pdf.CellFormat(30, 7, "Start date", "1", 0, "L", false, 0, "")
		pdf.CellFormat(30, 7, "End date", "1", 0, "L", false, 0, "")
		pdf.CellFormat(75, 7, "Purpose", "1", 0, "L", false, 0, "")
		pdf.CellFormat(39, 7, "Kilometers", "1", 1, "R", false, 0, "")
		pdf.SetFont("Arial", "", 9)
	}

	pdf.AddPage()
	writeHeader()
	writeTableHeader()
	for _, rec := range records {
		if pdf.GetY() > 265 {
			pdf.AddPage()
			writeHeader()
			writeTableHeader()
		}
		purpose := t(rec.Purpose)
		if len(purpose) > 48 {
			purpose = purpose[:45] + "..."
		}
		pdf.CellFormat(30, 7, rec.StartDate, "1", 0, "L", false, 0, "")
		pdf.CellFormat(30, 7, rec.EndDate, "1", 0, "L", false, 0, "")
		pdf.CellFormat(75, 7, purpose, "1", 0, "L", false, 0, "")
		pdf.CellFormat(39, 7, formatKM(rec.KilometersMilli), "1", 1, "R", false, 0, "")
	}

	pdf.Ln(6)
	pdf.SetFont("Arial", "B", 10)
	pdf.CellFormat(0, 6, fmt.Sprintf("Total mileage: %s km", formatKM(totalKM)), "", 1, "R", false, 0, "")
	pdf.SetFont("Arial", "", 9)
	for _, line := range breakdown {
		pdf.CellFormat(0, 5,
			fmt.Sprintf("%s km @ $%d.%02d/km: %s", formatKM(line.KilometersMilli), line.CentsPerKM/100, line.CentsPerKM%100, formatMoney(line.AmountCents)),
			"", 1, "R", false, 0, "")
	}
	pdf.Ln(1)
	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(0, 8, fmt.Sprintf("Total reimbursement: %s %s", formatMoney(totalCents), t(currency)), "T", 1, "R", false, 0, "")

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return GeneratedReport{}, err
	}

	filename := fmt.Sprintf("%d.%02d - %s - mileage.pdf", totalCents/100, totalCents%100, sanitizeFilename(localpart))
	return GeneratedReport{FileName: filename, PDF: buf.Bytes(), TotalCents: totalCents, TotalMilliKM: totalKM}, nil
}

func sanitizeFilename(s string) string {
	if s == "" {
		return "user"
	}
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("._-=", r) {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}
