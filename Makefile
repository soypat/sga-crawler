buildflags = -ldflags=""
binname = sgacrawl
distr:
	GOOS=windows GOARCH=amd64 go build ${buildflags} -o bin/${binname}.exe
	GOOS=linux GOARCH=amd64 go build ${buildflags} -o bin/${binname}_linux
	cp README.md README.txt
	zip ${binname} -j bin/${binname}.exe bin/${binname}_linux README.txt .sgacrawl.yaml
	rm README.txt
mkbin:
	mkdir bin

clean:
	rm plans.json
	rm classes.json