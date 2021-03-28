package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/gocolly/colly/v2"
	"github.com/spf13/viper"
	"golang.org/x/crypto/ssh/terminal"
)

const (
	domain        = "sga.itba.edu.ar"
	urlStart      = "https://" + domain
	htmlNameAttr  = "name"
	htmlValueAttr = "value"
	htmlLabelElem = "label"
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
	FilterLevel_EducacionEjecutiva = "3"
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

type htmlAction struct {
	query    string
	callback func(e *colly.HTMLElement)
}

func scrape() error {
	// we define the variables we want to change upon callback
	var cursosURL, careerURL string
	loginCallbacks := []htmlAction{
		{
			query: "ul.nav",
			callback: func(e *colly.HTMLElement) {
				classHref := e.ChildAttr("li:nth-of-type(3) ul.dropdown-menu li:nth-of-type(3) a", "href")
				if len(classHref) > 100 {
					cursosURL = trimDirectories(e.Request.URL.String(), 1) + "/" + classHref
				}
				careerHref := e.ChildAttr("li:nth-of-type(3) ul.dropdown-menu li:nth-of-type(2) a", "href")
				if len(careerHref) > 100 {
					careerURL = trimDirectories(e.Request.URL.String(), 1) + "/" + careerHref
				}
			}},
	}
	usr, pwd := readUserData()
	d, err := sgaLogin(usr, pwd, loginCallbacks)
	// delete user data once finished with login
	usr, pwd = "", ""
	if err != nil {
		return err
	}
	if viper.GetBool("scrape.careerPlans") {
		err = scrapeCareerPlans(d, careerURL)
	}
	// We proceed to set filter for courses of a certain year
	if viper.GetBool("scrape.classes") {
		err = scrapeClasses(d, cursosURL)
	}
	return err
}

func sgaLogin(usr, pwd string, actions []htmlAction) (*colly.Collector, error) {
	loginURI := make(map[string]string, 64)
	var actionURL, loginURL string
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
		return nil, err
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
	// start by visiting SGA
	err = c.Visit(urlStart)
	c.Wait()
	d := c.Clone()
	var userName string
	d.OnHTML("div.span6:nth-child(1) > span:nth-child(1)", func(e *colly.HTMLElement) {
		userName = e.Text
	})
	for _, v := range actions {
		d.OnHTML(v.query, v.callback)
	}
	postURL := trimDirectories(loginURL, 4) + trimDirectories(actionURL, -3) + "/"
	// LOGIN TO SGA (and trim jsessionid
	_ = d.Post(postURL[:strings.Index(postURL, ";")], loginURI)
	d.Wait()
	if userName == "" {
		return nil, fmt.Errorf("login unsuccesful. check login details")
	}
	logScrapef("[inf] logged in as %s", userName)
	return d, nil
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
	bytePassword, err := terminal.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", ""
	}
	pwd = string(bytePassword)
	// scanner.Scan()
	// pwd = scanner.Text()
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

func logScrape(args ...interface{}) {
	logScrapef("%s", args...)
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
	if viper.GetBool("log.toFile") {
		_, _ = logFile.WriteString(msg)
		_ = logFile.Sync()
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
