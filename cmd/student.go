/*
Copyright © 2020 NAME HERE <EMAIL ADDRESS>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"encoding/json"
	"fmt"
	wd "github.com/fedesog/webdriver"
	"github.com/spf13/cobra"
	"os"
	"strconv"
	"strings"
	"time"
)

// studentCmd represents the student command
var studentCmd = &cobra.Command{
	Use:   "student",
	Short: "Obtain student information. Requires chromedriver.exe",
	Long:  `Obtain student grades and credits.
Chrome driver can be obtained from https://chromedriver.chromium.org/

`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := scrapeStudent(args); err != nil {
			fmt.Print(err)
			os.Exit(1)
		}
	},
}
var driverPath string
func init() {
	rootCmd.AddCommand(studentCmd)

	// Here you will define your flags and configuration settings.
	studentCmd.Flags().StringVar(&driverPath,"driver","chromedriver.exe","Indicate chrome driver executable location. By default in working directory.")
	//studentCmd.PersistentFlags().String("driver", "chromedriver.exe", "Indicate chrome driver executable location. By default in working directory.")
	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// studentCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

}

type student struct {
	Classes []class
}

type grade struct {
	Number float64
	Date   string
	Pass   bool
	Record string `json:",omitempty"`
}

func scrapeStudent(_ []string) error {
	//usr,pwd := readUserData()
	Stu := student{Classes: []class{}}
	classCount := 0
	session, err := sgaDriverLogin()
	if err != nil {
		return err
	}
	session, err = mouseClickSelector(session, `#content > div.navbar > div > div > div > ul > li:nth-of-type(4)`)
	if err != nil {
		return err
	}
	session, err = mouseClickSelector(session, `#content > div.navbar > div > div > div > ul > li.dropdown.open > ul > li > a`)
	if err != nil {
		return err
	}
	time.Sleep(time.Second * 2)
	href := attrQuery(session, "href", `#content > div.backgroundBordered > div > div.tab-row > ul > li.tab3 > a`)
	//nextURL := urlStart+"/app2" + trimDirectories(href,-2)
	_ = session.Url(href)
	time.Sleep(time.Second * 1)
	var analyticURL string
	analyticURL = attrQuery(session, "href", `#content > div.backgroundBordered > div > div.tab-panel > div > div > div.tab-row > ul > li:nth-of-type(3) > a`)
	_ = session.Url(analyticURL) // analiticodeNotas URL
	loaded := false
	var elems = []wd.WebElement{}
	for !loaded {
		elems, err = querys(session, `#content > div.backgroundBordered > div > div.tab-panel > div > div > div.tab-panel > div > div.backgroundBordered > div:nth-child(6) > div:nth-child(1) > table > tbody > tr`)
		if err == nil && len(elems) > 0 {
			loaded = true
		}
	}
	logScrape("[scp] starting scraping student data")
	for _, e := range elems {
		rows, err := e.FindElements(wd.FindElementStrategy("css selector"), `table > tbody > tr`)
		if err != nil {
			return err
		}
		var Class class
		for _, r := range rows {
			tds, err := r.FindElements(wd.FindElementStrategy("css selector"), `td`)
			if err != nil {
				return err
			}
			if len(tds) < 3 {
				return fmt.Errorf("expecting 3 columns in table data")
			}
			classString, err := tds[0].Text()
			if err != nil {
				return err
			}
			grade1String, err := tds[1].Text()
			if err != nil {
				return err
			}
			grade2String, err := tds[2].Text()
			if err != nil {
				return err
			}
			if classString != "" {
				classString, credString := classString[:strings.LastIndex(classString, "(")], classString[1+strings.LastIndex(classString, "("):]
				creds, err := strconv.Atoi(strings.TrimRight(credString, " Créditos)\t"))
				if err != nil {
					return err
				}
				splitClassString := strings.Split(classString, "-")
				Class = class{
					Name:       strings.TrimSpace(splitClassString[1]),
					Code:       strings.TrimSpace(splitClassString[0]),
					Comissions: nil,
					Credits:    creds,
					Grades:     []grade{},
				}
				Stu.Classes = append(Stu.Classes, Class)
				classCount++
			}
			if grade1String != ""  {
				if strings.Index(strings.ToLower(grade1String),"aprobada")>=0 {
					Stu.Classes[classCount-1].Grades = append(Stu.Classes[classCount-1].Grades, grade{
						Pass: true,
						Record:grade1String,
						Number: -1,
					})
					continue
				}
				colorDiv, err := tds[1].FindElement("xpath", `div`)
				if err != nil {
					return err //continue // grade1String ==
				}
				style, err := colorDiv.GetAttribute("style")
				if err != nil {
					return err
				}
				splitString := strings.Split(grade1String, " ")
				thegrade, err := strconv.ParseFloat(strings.Replace(splitString[0], ",", ".", 1), 64)
				if err == nil {
					Stu.Classes[classCount-1].Grades = append(Stu.Classes[classCount-1].Grades, grade{
						Number: thegrade,
						Date:   strings.Join(splitString[1:], " "),
						Pass:   !strings.Contains(style,"red"),
						Record: "",
					})
				}
			}
			if grade2String != "" {
				grade2Divs, err := tds[2].FindElements("xpath", `div`)
				if err != nil {
					return err
				}
				for _, div := range grade2Divs {
					grade2Text, err := div.Text()
					if err != nil {
						return err
					}
					colorDiv, err := div.FindElement("xpath",`div`)
					if err != nil {
						return err
					}
					style, err := colorDiv.GetAttribute("style")
					splitString := strings.Split(grade2Text, " ")
					dateRecordString := strings.Join(splitString[1:],"")
					recordIndex := strings.Index(strings.ToLower(dateRecordString),"acta")
					if recordIndex < 1 {
						continue // Nota no consolidada
					}
					thegrade, err := strconv.ParseFloat(strings.Replace(splitString[0], ",", ".", 1), 64)
					if err == nil {
						Stu.Classes[classCount-1].Grades = append(Stu.Classes[classCount-1].Grades, grade{
							Number: thegrade,
							Date:   strings.TrimLeft(strings.TrimRight(dateRecordString[:recordIndex],") \t"),"( \t"),
							Pass:   !strings.Contains(style,"red"),
							Record: strings.TrimSpace(dateRecordString[recordIndex+len("acta:"):]),
						})
					}
				}
			}
		}
	}
	fo, _ := os.Create("student.json")
	defer fo.Close()
	b, _ := json.MarshalIndent(Stu, " ", "\t")
	fo.Write(b)
	fo.Sync()
	logScrape("[scp] finished scraping student data")
	return nil
}


func querys(s *wd.Session, querySelector string) ([]wd.WebElement, error) {
	return s.FindElements(wd.FindElementStrategy("css selector"), querySelector)
}

func query(s *wd.Session, querySelector string) (wd.WebElement, error) {
	return s.FindElement(wd.FindElementStrategy("css selector"), querySelector)
}

func sgaDriverLogin(_ ...string) (*wd.Session, error) {
	//driverPath, _ :=  studentCmd.PersistentFlags().GetString("driver")
	chromeDriver := wd.NewChromeDriver(driverPath)
	err := chromeDriver.Start()
	if err != nil {
		return &wd.Session{}, err
	}
	var session *wd.Session
	desired := wd.Capabilities{"Platform": "Windows"}
	required := wd.Capabilities{"Platform": "Windows"}
	session, err = chromeDriver.NewSession(desired, required)
	if err != nil {
		return session, err
	}
	err = session.Url(urlStart)
	if err != nil {
		return session, err
	}
	logScrape("[inf] please login")
	time.Sleep(time.Second * 10)
	for {
		time.Sleep(time.Second)
		usernameE, err := query(session, `#header > div > div.span6.pull-right > div > div > span`)
		if err != nil {
			continue
		}
		username, _ := usernameE.Text()
		if username != "" {
			logScrapef("[inf] logged in as %s", username)
			return session, nil
		}
	}
}

func attrQuery(s *wd.Session, attrName, querySelector string) string {
	e, err := query(s, querySelector)
	if err != nil {
		return ""
	}
	attribute, err := e.GetAttribute(attrName)
	if err != nil {
		return ""
	}
	return attribute
}
func mouseClickSelector(s *wd.Session, querySelector string) (*wd.Session, error) {
	var m wd.MouseButton
	elem, err := query(s, querySelector) // button selector
	if err != nil {
		return s, err
	}
	err = s.MoveTo(elem, 0, 0)
	if err != nil {
		return s, err
	}
	return s, s.Click(m)
}



// this is old colly code. New code shall use github.com/fedesog/webdriver
/*
func scrapeStudent() error {
	usr, pwd := readUserData()
	var cursosURL, careerURL, legacyURL string
	loginCallbacks := []htmlAction{{
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
			legacyHref := e.ChildAttr("li:nth-of-type(4) ul.dropdown-menu li:nth-of-type(1) a", "href")
			if len(legacyHref) > 100 {
				legacyURL = trimDirectories(e.Request.URL.String(), 1) + "/" + legacyHref
			}
		}},
	}
	d, err := sgaLogin(usr, pwd, loginCallbacks)
	if err != nil {
		return err
	}

	leg := d.Clone()
	var studentDataURL, academicHistoryURL, gradeAnalyticURL  string
	leg.OnHTML("#content > div.backgroundBordered > div > div.tab-row > ul > li.tab3 > a", func(e *colly.HTMLElement) {
		studentDataURL = urlStart + "/app2" + trimDirectories(e.Attr("href"), -2)
	})
	leg.Visit(legacyURL)
	leg.Wait()
	logScrapef("scraped url %s",studentDataURL)
	leg = leg.Clone()
	leg.OnHTML("#content > div.backgroundBordered > div > div.tab-panel > div > div > div.tab-row > ul > li.tab2 > a", func(e *colly.HTMLElement) {
		gradeAnalyticURL = urlStart + "/app2" + trimDirectories(e.Attr("href"), -2)
	})
	leg.OnHTML("#content > div.backgroundBordered > div > div.tab-panel > div > div > div.tab-row > ul > li.tab3 > a", func(e *colly.HTMLElement) {
		academicHistoryURL = urlStart + "/app2" + trimDirectories(e.Attr("href"), -2)
	})
	leg.Visit(studentDataURL)
	leg.Wait()
	logScrapef("scraped url %s",gradeAnalyticURL)
	logScrapef("scraped url %s",academicHistoryURL)
	_,err = scrapeGradeAnalytic(leg.Clone(),gradeAnalyticURL)
	if err != nil {
		return err
	}
	return nil
}

type Student struct{

}

func scrapeGradeAnalytic(c *colly.Collector, url string) (interface{},error) {
	var academicThroughput, linAcademicThroughput, ajaxGet string
	c.OnHTML("head > script:nth-child(19)", func(scriptElement *colly.HTMLElement) {
		idx1, idx2 := strings.Index(scriptElement.Text,"wicketAjaxGet('."), strings.Index(scriptElement.Text,"',function() {")
		ajaxGet = fmt.Sprintf("%s/app2%s?random=%1.16f",urlStart,scriptElement.Text[idx1:idx2],0.4232312319658345)
	})
	c.Visit(url)
	c.Wait()
	c = c.Clone()
	c.OnResponse(func(r *colly.Response) {
		fmt.Printf("%s", r.Body)
	})
	//c.OnHTML("html", func(e *colly.HTMLElement) {
	//	time.Sleep(5*time.Second)
	//})
	//c.OnHTML("html",writeHTMLToFile)
	//c.OnHTML("#id59f", func(e *colly.HTMLElement) {
	//
	//	academicThroughput = e.ChildText("> div:nth-child(5) > p > span:nth-child(2)")
	//	linAcademicThroughput = e.ChildText("> div:nth-child(6) > p > span:nth-child(2)")
	//})
	//c.OnHTML("#id51 > div:nth-child(1) > table:nth-child(1)", func(e *colly.HTMLElement) {
	//	e.ForEach("> tbody > tr > td > div", func(i int, eSem *colly.HTMLElement) {
	//		semesterName := eSem.ChildText(">h6")
	//		logScrapef("%s",semesterName)
	//	})
	//})

	c.Visit(ajaxGet)
	c.Wait()
	logScrapef("grades %s, %s", academicThroughput, linAcademicThroughput)
	return nil,nil
}
*/
