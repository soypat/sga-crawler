# sgacrawl (pronounced *shjuhrahl*)
SGA crawler CLI applet

Run by opening command line and executing `sgacrawl.exe -h`. Help will be shown and an example `.sgacrawl.yaml` configuration file will be shown.

This configuration file should be located in operating directory.

sgacrawl can create three files: 

* `classes.json`  Will have class crawling results
* `plans.json`  Will have career plan crawling results
* `student.json` Using the `student` command one can obtain grades, passed classes and such
* `sgacrawl.log`  if log.tofile  is set to true this file shall contain log information

each execution will overwrite previous file.