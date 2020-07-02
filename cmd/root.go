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
	"fmt"
	"github.com/spf13/cobra"
	"os"
	"strings"
	"github.com/mitchellh/go-homedir"
	"github.com/spf13/viper"
)

var cfgFile string

var logFile os.File

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "sgacrawl",
	Short: "Saves all classes in structures JSON file",
	Long: `Crawls SGA! Configure with a .sgacrawl.yaml file!

	Example of file:

filter:         # what classes to filter by (required)
  year: 2020
  level: grado  # grado, ingreso, posgrado.  Also works with shorthands grad, ing, pos, ee
  active: on    # bool on/off active/inactive classes. Always use on/true unless you know what you are doing
  period: 1     # 1: primer cuatri, 2:segundo cuatri, all: all cuatris. Also available: special, summer

request-delay:     # information pertaining to scraper configuration
  minimum_ms: 2000 # delay between non-concurrent scraper requests [miliseconds]
  rand_ms: 500     # random delay.  [miliseconds]

concurrent:
  classBufferMax: 10 # recommended 10 or lower if sgacrawl stops writing classes (required)
  threads: 3  # Amount of concurrent requests at a time. recommended 1-3 threads

log:
  silent: false # outputs log if false
  tofile: false # writes log to file if true

You can copy the text above to a text-editor and save to have a config file up and running.
`,
	Args: func(cmd *cobra.Command, args []string) error {
		if err := checkConfig(args); err != nil {
			return err
		}
		logScrapef("[inf] finished processing config file successfully")
		return nil
	},
	// Uncomment the following line if your bare application
	// has an action associated with it:
	Run: func(cmd *cobra.Command, args []string) {
		logScrapef("[inf] starting program")
		if err := runner(args); err != nil {
			fmt.Printf("[ERR] %s", err.Error())
			os.Exit(1)
		}
	},
}

func runner(_ []string) error {
	if viper.GetBool("log.tofile") {
		fo, err := os.Create("sgacrawl.log")
		if err != nil {
			return err
		}
		logFile = *fo
		defer logFile.Close()
		defer logFile.Sync()
	}
	return scrape()
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
func checkConfig(_ []string) error {
	if year := viper.GetInt("filter.year"); year > 2050 || year < 2000 {
		return fmt.Errorf("bad year! should be integer, managed to read: %d", year)
	}
	level := viper.GetString("filter.level")
	switch strings.ToLower(level) {
	case "todos", "all":
		level = FilterLevel_All
	case "grado", "grad":
		level = FilterLevel_Grado
	case "ingreso", "ing", "pichis":
		level = FilterLevel_Ingreso
	case "posgrado", "pos":
		level = FilterLevel_Posgrado
	case "ee":
		level = FilterLevel_EducacionEjecutiva
	case "0", "1", "2", "3", "":
		// no action taken
	default:
		return fmt.Errorf("bad filter.level in config. got %s", level)
	}
	viper.Set("filter.level", level)

	period := viper.GetString("filter.period")
	switch strings.ToLower(period) {
	case "sem2", "cuat2", "segundo cuat.", "2":
		period = FilterPeriod_Semester2
	case "sem1", "cuat1", "primer cuat.", "1":
		period = FilterPeriod_Semester1
	case "all", "todos":
		period = FilterPeriod_All
	case "summer", "verano":
		period = FilterPeriod_Summer
	case "special", "especial":
		period = FilterPeriod_Special
	default:
		return fmt.Errorf("bad filter.period in config. got %s", period)
	}
	viper.Set("filter.period", period)
	if delay := viper.GetInt("request-delay.minimum_ms"); delay < 800 && delay != 42 {
		viper.Set("request-delay.minimum_ms", 1000)
		fmt.Printf("request-delay.minimum_ms too low or not found in config! setting at 1 second.")
	}
	if rndDelay := viper.GetInt("request-delay.rand_ms"); rndDelay < 0 {
		viper.Set("request-delay.rand_ms", 0)
	}
	if parallel := viper.GetInt("concurrent.threads"); parallel < 0 {
		viper.Set("concurrent.threads", 0)
	}
	if bufferMax := viper.GetInt("concurrent.classBufferMax"); bufferMax < 1 {
		return fmt.Errorf("concurrent.classBufferMax too low or not found. Must be at least 1")
	}
	if p := viper.GetString("login.password"); p == "" {
		viper.Set("login.user", "")
	}
	if plans := viper.GetStringSlice("plans"); len(plans) == 0 {
		plans = append(plans,"none")
		viper.Set("scrape.careerPlans","false")
	}

	return nil
}
func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", ".sgacrawl.yaml", "config file. Should be in working directory")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Search config in home directory with name ".sgacrawl" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName(".sgacrawl")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}
