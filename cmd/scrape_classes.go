package cmd

import (
	"encoding/json"
	"fmt"
	"github.com/gocolly/colly/v2"
	"github.com/spf13/viper"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

func scrapeClasses(c *colly.Collector, classesURL string) error {
	var actionURL string
	col := c.Clone()
	var filterURI = make(map[string]string, 64)
	col.OnHTML("form", func(e *colly.HTMLElement) {
		actionURL = e.Attr("action")
		// fill out all required form inputs including hidden inputs and selects
		e.ForEach("input,select", func(i int, element *colly.HTMLElement) {
			var filterNumber int
			key, value := element.Attr(htmlNameAttr), element.Attr("value")
			idx := strings.LastIndex(key, ":filter:filter")
			if !(idx < 1) {
				filterNumber, _ = strconv.Atoi(key[idx-1 : idx])
			}
			switch filterNumber {
			case filterYear:
				filterURI[key] = viper.GetString("filter.year")
			case filterLevel:
				filterURI[key] = viper.GetString("filter.level")
			case filterPeriod:
				filterURI[key] = viper.GetString("filter.period")
			case filterActive:
				filterURI[key] = FilterActive_Unchecked
				if viper.GetBool("filter.active") {
					filterURI[key] = FilterActive_Checked
				}
			default:
				if key != "" {
					filterURI[key] = value
				}
			}
		})
	})
	_ = col.Visit(classesURL)
	col.Wait()
	scrapeBase := col.Clone()
	var scrapeBaseURL string
	scrapeBase.OnHTML("html", func(e *colly.HTMLElement) {
		scrapeBaseURL = e.Request.URL.String()
	})
	filterPostURL := urlStart + "/app2" + trimDirectories(actionURL, -3) + "/"
	_ = scrapeBase.Post(filterPostURL, filterURI)
	scrapeBase.Wait()
	if scrapeBaseURL == "" {
		return fmt.Errorf("got no url. check user/password settings")
	}

	// Initialize scraper and class queue
	scraper := scrapeBase.Clone()
	urlClassQueue := make(chan string, viper.GetInt("concurrent.classBufferMax"))
	var nextURL string
	var pageNo int
	// send request to scrape class to channel
	// count class link sites
	var scrapedCounter = 0
	keepClassCount := func(_ *colly.Response) {
		scrapedCounter++
		logScrapef("[scp](%d) class link scrape...", scrapedCounter)
	}
	scraper.OnResponse(keepClassCount)
	scraper.OnHTML("form table tbody tr td:last-of-type span span a", func(e *colly.HTMLElement) {
		href := trimDirectories(e.Attr("href"), -3)
		for {
			if len(urlClassQueue) == cap(urlClassQueue) {
				// url channel full
				time.Sleep(2000 * time.Millisecond)
				continue
			}
			break
		}
		urlClassQueue <- urlStart + "/app2" + href
	})
	// obtain next url to find next set of classes
	scraper.OnHTML("form table thead div.navigator span a.next", func(e *colly.HTMLElement) {
		nextURL = urlStart + "/app2" + trimDirectories(e.Attr("href"), -3)
	})
	scraper.OnHTML("form table thead div.navigator span.next", func(e *colly.HTMLElement) {
		nextURL = ""
	})
	nextURL = scrapeBaseURL
	EOS := false
	var wg sync.WaitGroup

	go traverseClasses(scraper, &urlClassQueue, &wg)
	for !EOS {
		pageNo++
		_ = scraper.Visit(nextURL)
		scraper.Wait()
		if nextURL == "" {
			urlClassQueue <- "EOS" // end class traversal
			EOS = true
		}
	}
	logScrapef("[inf] finished scraping class links")
	wg.Wait()
	logScrapef("[inf] program end")
	return nil
}

func traverseClasses(s *colly.Collector, c *chan string, wg *sync.WaitGroup) {
	wg.Add(1)
	defer wg.Done()
	urlComissionQueue := make(chan string, viper.GetInt("concurrent.classBufferMax")*3)
	baseClassScraper := s.Clone()
	go traverseComissions(s, &urlComissionQueue, wg)
	for {
		url, ok := <-*c
		if !ok {
			time.Sleep(1e7)
			continue
		}
		if url == "EOS" {
			urlComissionQueue <- "EOS"
			logScrapef("[inf] finished classes")
			break
		}
		classScraper := baseClassScraper.Clone()
		//classScraper.OnHTML("html", writeHTMLToFile)
		classScraper.OnHTML("li.tab1 a", func(e *colly.HTMLElement) {
			urlComissionQueue <- urlStart + "/app2" + trimDirectories(e.Attr("href"), -2)
		})
		logScrapef("[scp] visiting class...")
		_ = classScraper.Visit(url)
		classScraper.Wait()
	}
}

type class struct {
	Name       string
	Code       string
	Comissions []comission `json:",omitempty"`
	Credits    int         `json:",omitempty"`
	Grades     []grade     `json:",omitempty"`
}
type comission struct {
	Label     string
	Schedules []string
	Teachers  []string
	Location  string
}

//type weekday int

//const (
//	dayMon weekday = iota
//	dayTue
//	dayWed
//	dayThu
//	dayFri
//	daySat
//	daySun
//)

func traverseComissions(s *colly.Collector, c *chan string, wg *sync.WaitGroup) {
	CLASSES := make(chan class, viper.GetInt("concurrent.classBufferMax"))
	baseComissionScraper := s.Clone()
	go writeClasses(&CLASSES, wg)
	for {
		url, ok := <-* c
		if !ok {
			time.Sleep(1e7)
			continue
		}
		if url == "EOS" {
			CLASSES <- class{
				Comissions: nil,
				Name:       "EOS",
				Code:       "EOS",
			}
			logScrapef("[inf] finished comissions")
			return
		}
		comissionScraper := baseComissionScraper.Clone()
		comissionScraper.OnHTML("div.tab-panel", func(eclass *colly.HTMLElement) {
			var newClass class
			newClass.Code = eclass.ChildText("h4 span:first-of-type")
			newClass.Name = eclass.ChildText("h4 span:nth-of-type(2)")
			eclass.ForEach("table tbody tr", func(i int, ecom *colly.HTMLElement) {
				var newCom comission
				ecom.ForEach("td", func(i int, comCol *colly.HTMLElement) {
					switch i {
					case 0: // schedule Label
						newCom.Label = comCol.ChildText(htmlLabelElem)
					case 1: // schedule column
						comCol.ForEach("div", func(schedRow int, schedItem *colly.HTMLElement) {
							var spanItems []string
							schedItem.ForEachWithBreak("span", func(scheduleSpan int, span *colly.HTMLElement) bool {
								if scheduleSpan == 3 {
									newCom.Location = strings.TrimSpace(span.Text)
									return false
								}
								spanItems = append(spanItems, strings.TrimSpace(span.Text))
								return true
							})
							newCom.Schedules = append(newCom.Schedules,
								strings.Join(spanItems, "; "))
						})
					case 2: // Teachers
						newCom.Teachers = comCol.ChildTexts("div " + htmlLabelElem)
					}
				})
				newClass.Comissions = append(newClass.Comissions, newCom)
			})
			CLASSES <- newClass
		})
		logScrapef("[scp] visiting comission...")
		_ = comissionScraper.Visit(url)
		comissionScraper.Wait()
	}
}

func writeClasses(c *chan class, wg *sync.WaitGroup) {
	wg.Add(1)
	defer wg.Done()
	fo, err := os.Create("classes.json")
	if err != nil {
		panic("Could not create class file. Permissions ok?")
	}
	defer fo.Close()
	defer fo.Sync()
	_, _ = fo.WriteString("[\n")
	defer time.Sleep(time.Nanosecond)
	defer fo.WriteString("\n]")
	classCounter := 0
	for {
		class, ok := <-*c
		if !ok {
			time.Sleep(1e7)
			_ = fo.Sync()
			continue
		}
		if class.Name == "EOS" {
			logScrapef("[out] finished writing classes")
			break
		}
		if class.Comissions == nil {
			continue
		}
		if classCounter != 0 {
			_, _ = fo.Write([]byte(",\n"))
		}
		var theBytes []byte
		if viper.GetBool("minify") {
			theBytes, _ = json.Marshal(class)
		} else {
			theBytes, _ = json.MarshalIndent(class, viper.GetString("beautify.prefix"), viper.GetString("beautify.indent"))
		}
		if _, err = fo.Write(theBytes); err != nil {
			panic("error writing to class file")
		}
		_ = fo.Sync()
		logScrapef("[out] class %s written to file", class.Name)
		classCounter++
	}
}
