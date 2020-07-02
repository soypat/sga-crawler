package main

import (
	"fmt"
	"github.com/gocolly/colly/v2"
	"os"
	"time"
)

const gistURL = "https://gist.githubusercontent.com/soypat/df050b25339b5d2cb2d1d2293013c92c/raw/bcc9f8083c919350c38977ebdd819e4ebeeb1be2/collyBug.html"
const jbURL = "http://localhost:63342/sgacrawl/collytest/index.html"
const domain = "localhost:8080"
const bugHtml = "http://"+domain
func main() {
	selector := `html body div.container div.row div.backgroundBordered h4:first-of-type span`
	//selector := `#content div h4: span` // both selectors yield the same unexpected result
	c := colly.NewCollector(
		colly.AllowURLRevisit(),
		colly.Async(false),
	)
	var plan string
	c.OnResponse(func(_ *colly.Response) {
		fmt.Printf("got response")
	})
	c.OnHTML("html",writeHTMLToFile)
	c.OnHTML(selector , func(ele *colly.HTMLElement) {
		plan= ele.Text
	})
	err := c.Visit(gistURL)
	if err != nil {
		fmt.Print(err)
	}
	c.Wait()
	time.Sleep(3e9)
	fmt.Print(plan)
}


var htmlCounter = 0
// for debugging purposes. Pass as argument to as
// collyCollector.OnHTML('html', writeHTMLToFile)
// to write whole HTML file to scraped/ directory
// The scraped/ directory must be created beforehand
func writeHTMLToFile(e *colly.HTMLElement) {
	htmlCounter++
	fo, _ := os.Create(fmt.Sprintf("out%d.html", htmlCounter))
	_, _ = fo.Write(e.Response.Body)
	_ = fo.Close()
}
