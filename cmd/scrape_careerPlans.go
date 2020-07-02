package cmd

import (
	"fmt"
	"github.com/gocolly/colly/v2"
	"github.com/spf13/viper"
	"strconv"
	"strings"
	"sync"
)

const (
	filterCareerLevel  = 4
	filterCareerActive = 5
)

func scrapeCareerPlans(c *colly.Collector, careerURL string) error {
	var actionURL string
	col := c.Clone()
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
			case filterCareerLevel:
				filterURI[key] = viper.GetString("filter.level")
			case filterCareerActive:
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
	_ = col.Visit(careerURL)
	col.Wait()
	scrapeBase := col.Clone()
	var scrapeBaseURL string
	var careerLinks []string
	keepCareerCount := func(_ *colly.Response) {
		logScrapef("[scp] career link scrape...")
	}
	scrapeBase.OnResponse(keepCareerCount)
	scrapeBase.OnHTML("html", func(e *colly.HTMLElement) {
		scrapeBaseURL = e.Request.URL.String()
	})
	scrapeBase.OnHTML("form table tbody tr td:last-of-type span span:nth-of-type(4) a", func(e *colly.HTMLElement) {
		careerLinks = append(careerLinks, urlStart+"/app2"+trimDirectories(e.Attr("href"), -3))
	})
	filterPostURL := urlStart + "/app2" + trimDirectories(actionURL, -3) + "/"
	_ = scrapeBase.Post(filterPostURL, filterURI)
	scrapeBase.Wait()
	if scrapeBaseURL == "" || len(careerLinks) == 0 {
		return fmt.Errorf("got no url in career links. check user/password settings")
	}
	// create the scraper to scrape all classes
	var wg sync.WaitGroup
	wg.Add(len(careerLinks))
	for _, l := range careerLinks {
		go traverseCareer(scrapeBase.Clone(), l, &wg)
	}
	wg.Wait()
	// TODO write plans to file
	return nil
}

func traverseCareer(c *colly.Collector, careerURL string, group *sync.WaitGroup) {
	defer group.Done()
	plans := viper.GetStringSlice("plans") // these are already trimmed
	planMap := make(map[string]string, 100)
	var careerName string
	c.OnHTML("#content p:first-of-type span.value",
		func(e *colly.HTMLElement) { careerName = e.Text })
	c.OnHTML("table tbody tr", func(e *colly.HTMLElement) {
		plan := e.ChildText("td:first-of-type span")
		href := e.ChildAttr("td:last-of-type a", "href")
		if sliceContainsIdx(plans, plan) >= 0 || plans[0] == "all" {
			planMap[plan] = urlStart + "/app2" + trimDirectories(href, -2)
		}
	})

	_ = c.Visit(careerURL)
	c.Wait()
	var Plans []*plan
	for name, link := range planMap {
		Plan, err := traversePlan(c.Clone(), link)
		if err != nil {
			logScrapef("[err] could not scrape plan %s. got error %s", name, err)
		} else {
			Plans = append(Plans, Plan)
		}
	}
}

func traversePlan(c *colly.Collector, careerURL string) (*plan, error) {
	var Plan plan
	//c.OnHTML("html", writeHTMLToFile) //"#content div h4: span"
	// TODO wait for colly to fix this bug
	c.OnHTML(`html body div.container div.row div.backgroundBordered h4:first-of-type span`, func(ele *colly.HTMLElement) {
		Plan.Name = ele.Text
	})
	c.OnHTML("table:first-of-type tbody tr", func(e *colly.HTMLElement) {
		var Semester semester = make(map[string][]course, 20)
		semesterName := e.ChildText("td div div.row h4 span")
		e.ForEach("td div div table tbody tr", func(_ int, element *colly.HTMLElement) {
			var Course course
			codeAndName := strings.Split(element.ChildText("td:first-of-type a span"), "-")
			if len(codeAndName) != 2 {
				return
			}
			Course.Code = strings.TrimSpace(codeAndName[0])
			Course.Name = strings.TrimSpace(codeAndName[1])
			fmt.Print(Plan.Name,semesterName,Course.Name,"\n")
			Course.Credits = element.ChildText("td:nth-of-type(2)")
			Course.ReqCredits = element.ChildText("td:nth-of-type(3)")
			element.ForEach("td:last-of-type span", func(_ int, element2 *colly.HTMLElement) {
				Course.Correlatives = append(Course.Correlatives, element2.Text)
			})
			Semester[semesterName] = append(Semester[semesterName], Course)
		})
		Plan.semesters = append(Plan.semesters, Semester)
	})
	_ = c.Visit(careerURL)
	c.Wait()
	if Plan.Name == "" {
		return nil, fmt.Errorf("could not read career plan")
	}
	logScrapef("[scp] scraped plan %s", Plan.Name)
	return &Plan, nil
}

type plan struct {
	Name      string
	semesters []semester `json:"-"`
}

type semester map[string][]course

type course struct {
	Name         string
	Code         string
	Credits      string
	ReqCredits   string
	Correlatives []string
}

// returns -1 if string is not contained in slice, returns idx otherwise
func sliceContainsIdx(sli []string, s string) int {
	for i, v := range sli {
		if s == v {
			return i
		}
	}
	return -1
}
