package cmd

import (
	"bufio"
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

const (
	// filter option identificator
	filterLevel  = 3 // dropdown or 'select' element
	filterPeriod = 5 // dropdown or 'select' element
	filterYear   = 6 // text 'input' element
	filterActive = 9
	// URI header options
	FilterLevel_All                = ""
	FilterLevel_Ingreso            = "0"
	FilterLevel_Grado              = "1"
	FilterLevel_Posgrado           = "2"
	FilterLevel_EducacionEjecutiva = "2"
	// Period
	FilterPeriod_All       = ""
	FilterPeriod_Semester1 = "0"
	FilterPeriod_Semester2 = "1"
	FilterPeriod_Summer    = "2"
	FilterPeriod_Special   = "3"
	// Active/Inactive
	FilterActive_Checked   = "on"
	FilterActive_Unchecked = ""
)

var scrapedCounter = 0

var filterURI = make(map[string]string, 64)

const (
	domain        = "sga.itba.edu.ar"
	urlStart      = "https://" + domain
	htmlNameAttr  = "name"
	htmlValueAttr = "value"
	htmlLabelElem = "label"
)

func scrape() error {
	// loginURI info
	loginURI := make(map[string]string, 64)
	var actionURL, loginURL string
	var usr, pwd string
	//var err error
	usr, pwd = readUserData()
	c := colly.NewCollector(
		// Restrict crawling to specific domains
		colly.AllowedDomains(domain),
		// Allow visiting the same page multiple times
		colly.AllowURLRevisit(),
		// Allow crawling to be done in parallel / async
		colly.Async(true),
	)
	err := c.Limit(&colly.LimitRule{
		// Filter domains affected by this rule
		DomainGlob: domain,
		// Set a delay between requests to these domains
		Delay: time.Duration(viper.GetInt("request-delay.minimum_ms") * 1e6),
		// Add an additional random delay
		RandomDelay: time.Duration(viper.GetInt("request-delay.rand_ms") * 1e6),
		Parallelism: viper.GetInt("concurrent.threads"),
	})
	if err != nil {
		return err
	}
	// fill out loginURI form info for POST method
	c.OnHTML("input", func(e *colly.HTMLElement) {
		var value string
		inputName := e.Attr(htmlNameAttr)
		switch inputName {
		case "user", "username":
			value = usr
		case "password":
			value = pwd
		case "js":
			value = "1"
		default:
			value = e.Attr(htmlValueAttr)
		}
		loginURI[inputName] = value
		if inputName == "password" && len(value) > 0 {
			value = "********"
		}
		logScrapef("[loginURI] value:key -> %s:%s", inputName, value)
	})
	// look for form action attribute for post method
	c.OnHTML("form", func(e *colly.HTMLElement) {
		actionURL = e.Attr("action")
		loginURL = e.Request.URL.String()
	})
	// start visiting
	err = c.Visit(urlStart)
	c.Wait()
	d := c.Clone()
	var cursosURL string
	// Obtain course scraping site once we arrive at sga main page
	d.OnHTML("ul.nav", func(e *colly.HTMLElement) {
		cursos := e.ChildAttr("li:nth-of-type(3) ul.dropdown-menu li:nth-of-type(3) a", "href")
		if len(cursos) > 100 {
			cursosURL = trimDirectories(e.Request.URL.String(), 1) + "/" + cursos
		}
	})
	postURL := trimDirectories(loginURL, 4) + trimDirectories(actionURL, -3) + "/"
	// trim jsessionid
	scidx := strings.Index(postURL, ";")
	// LOGIN TO SGA
	_ = d.Post(postURL[:scidx], loginURI)
	d.Wait()
	// delete user data once finished with login
	usr, pwd = "", ""
	// We proceed to set filter for courses of a certain year
	filter := d.Clone()
	filter.OnHTML("form", func(e *colly.HTMLElement) {
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
	_ = filter.Visit(cursosURL)
	filter.Wait()
	scrapeBase := filter.Clone()
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

	// Initialize scraper queues
	//addedCourseURLs :=  make(map[string]string,1000)
	scraper := scrapeBase.Clone()
	urlClassQueue := make(chan string, viper.GetInt("concurrent.classBufferMax"))
	var nextURL string
	var pageNo int
	// send request to scrape class to channel
	scraper.OnResponse(keepCount)
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
	if err != nil {
		return err
	}
	return err
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
	Comissions []comission
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
		theBytes, _ := json.Marshal(class)
		if _, err = fo.Write(theBytes); err != nil {
			panic("error writing to class file")
		}
		_ = fo.Sync()
		logScrapef("[out] class %s written to file", class.Name)
		classCounter++
	}
}

func readUserData() (usr, pwd string) {
	usr, pwd = viper.GetString("login.user"), viper.GetString("login.password")
	if usr != "" {
		return usr, pwd
	}
	fmt.Print("user:")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	usr = scanner.Text()
	fmt.Print("password:")
	scanner.Scan()
	pwd = scanner.Text()
	return usr, pwd
}

func trimDirectories(path string, n int) string {
	if n < 0 {
		for i := 0; i < -n; i++ {
			path = strings.TrimPrefix(path, "/")
			idx := strings.Index(path, "/")
			path = path[idx:]
		}
		return path
	}
	for i := 0; i < n; i++ {
		path = strings.TrimSuffix(path, "/")
		idx := strings.LastIndex(path, "/")
		path = path[:idx]
	}
	return path
}
func keepCount(_ *colly.Response) {
	scrapedCounter++
	logScrapef("[scp](%d) class link scrape...", scrapedCounter)
}

func logScrapef(format string, args ...interface{}) {
	var msg string
	if len(args) == 0 {
		msg = fmt.Sprintf(format)
	} else {
		msg = fmt.Sprintf(format, args...)
	}
	msg = strings.TrimSuffix(msg, "\n") + "\n"
	if !viper.GetBool("log.silent") {
		fmt.Print(msg)
	}
	if viper.GetBool("log.tofile") {
		_, _ = logFile.WriteString(msg)

	}
}

var htmlCounter = 0

// for debugging purposes. Pass as argument to as
// collyCollector.OnHTML('html', writeHTMLToFile)
// to write whole HTML file to scraped/ directory
// The scraped/ directory must be created beforehand
func writeHTMLToFile(e *colly.HTMLElement) {
	htmlCounter++
	fo, _ := os.Create(fmt.Sprintf("scraped/out%d.html", htmlCounter))
	_, _ = fo.Write(e.Response.Body)
	_ = fo.Close()
}

/* goQuery basics cheat-sheet
$( '#header' ); // select the element with an ID of 'header'
$( 'li' );      // select all list items on the page
$( 'ul li' );   // select list items that are in unordered lists
$( '.person' ); // select all elements with a class of 'person'
$( 'li:nth-of-type(2)') // select second element
*/
