buildflags = -ldflags="-s -w" -i
binname = sgacrawl
distr:
	go build ${buildflags} -o bin/${binname}.exe
	cp README.md README.txt
	zip ${binname} -j bin/${binname}.exe README.txt .sgacrawl.yaml

mkbin:
	mkdir bin
