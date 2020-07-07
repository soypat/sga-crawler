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
)

const (
	filterCareerLevel  = 4
	filterCareerActive = 5
)

func scrapeCareerPlans(c *colly.Collector, careerURL string) error {
	var actionURL string
	var filterURI = make(map[string]string, 64)
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
	var Careers []career
	for _, l := range careerLinks {
		go traverseCareer(scrapeBase.Clone(), l, &wg, &Careers)
	}
	wg.Wait()
	b,err := json.MarshalIndent(Careers, viper.GetString("beautify.prefix") , viper.GetString("beautify.indent"))
	if viper.GetBool("minify") {
		b, err = json.Marshal(Careers)
	}
	if err != nil {
		panic(err)
	}
	fo, err := os.Create("plans.json")
	if err != nil {
		panic(err)
	}
	defer fo.Close()
	fo.Write(b)
	fo.Sync()
	logScrapef("[out] finished writing career plans to file")
	return nil
}

func traverseCareer(c *colly.Collector, careerURL string, group *sync.WaitGroup, Careers *[]career) {
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
	if len(Plans) == 0 {
		return
	}
	Career := career{
		Name:  careerName,
		Plans: Plans,
	}
	*Careers = append(*Careers, Career)
}

func traversePlan(c *colly.Collector, careerURL string) (*plan, error) {
	var Plan plan
	var Sems Semesters = make(map[string][]course, 64)
	c.OnHTML(`#content > div.backgroundBordered > h4 > span`, func(ele *colly.HTMLElement) {
		Plan.Name = ele.Text
	})
	c.OnHTML("table:first-of-type", func(e *colly.HTMLElement) {
		e.ForEach(`tbody tr`, func(i int, esem *colly.HTMLElement) {
			semesterName := esem.ChildText("tbody > tr > td div div.row h4 span")
			if semesterName == "Contenido:" {
				semesterName = e.ChildText(`thead > tr > th > div > div:nth-of-type(2) > span:first-of-type`)
			}
			esem.ForEach("td div div table tbody tr", func(_ int, ecourse *colly.HTMLElement) {
				var Course course
				codeAndName := strings.Split(ecourse.ChildText("td:first-of-type a span"), "-")
				if len(codeAndName) != 2 {
					return
				}
				Course.Code = strings.TrimSpace(codeAndName[0])
				Course.Name = strings.TrimSpace(codeAndName[1])
				Course.Credits = ecourse.ChildText("td:nth-of-type(2)")
				Course.ReqCredits = ecourse.ChildText("td:nth-of-type(3)")
				ecourse.ForEach("td:last-of-type > span", func(_ int, element2 *colly.HTMLElement) {
					Course.Correlatives = append(Course.Correlatives, strings.TrimSpace(element2.Text))
				})
				Sems[semesterName] = append(Sems[semesterName], Course)
			})
		})
	})
	_ = c.Visit(careerURL)
	c.Wait()
	if len(Sems) != 0 {
		Plan.Semesters = Sems
	}
	if Plan.Name == "" {
		return nil, fmt.Errorf("could not read career plan")
	}
	logScrapef("[scp] scraped plan %s", Plan.Name)
	return &Plan, nil
}

type career struct {
	Name  string
	Plans []*plan
}
type plan struct {
	Name      string
	Semesters
}

type Semesters map[string][]course

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
