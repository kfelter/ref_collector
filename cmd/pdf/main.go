package main

import (
	"io"
	"log"
	"os"

	pdf "github.com/SebastiaanKlippert/go-wkhtmltopdf"
)

func URLasPDF(url string, out io.Writer) error {

	// Create new PDF generator
	pdfg, err := pdf.NewPDFGenerator()
	if err != nil {
		panic(err)
	}

	// Write buffer contents to file on disk
	pdfg.SetOutput(out)

	// Set global options
	pdfg.Dpi.Set(300)
	pdfg.Orientation.Set(pdf.OrientationPortrait)
	pdfg.Grayscale.Set(false)

	// Create a new input page from an URL
	page := pdf.NewPage(url)

	// Set options for this page
	page.FooterRight.Set("[page]")
	page.FooterFontSize.Set(10)
	page.Zoom.Set(0.95)

	// Add to document
	pdfg.AddPage(page)

	// Create PDF document in internal buffer
	err = pdfg.Create()
	if err != nil {
		panic(err)
	}
	return nil
}

func main() {
	log.Println(URLasPDF("https://kfelter.github.io/", os.Stdout))
}
