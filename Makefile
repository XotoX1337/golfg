build:
	go mod tidy
	go build -ldflags "-s -w" -o dist/ ./cmd/golfg

esbuild:
	esbuild --bundle --minify assets/css/style.css --outfile=web/static/css/app.min.css --loader:.woff=file --loader:.woff2=file --loader:.eot=file --loader:.ttf=file --loader:.svg=file

run:
	go run ./cmd/golfg

clean:
	go clean

compile:
	go mod tidy
	GOOS=linux GOARCH=arm go build -ldflags "-s -w" -o dist/golfg-linux-arm ./cmd/golfg
	GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o dist/golfg-linux-amd64 ./cmd/golfg
	GOOS=windows GOARCH=amd64 go build -ldflags "-s -w" -o dist/golfg-windows-amd64.exe ./cmd/golfg
	GOOS=darwin GOARCH=amd64 go build -ldflags "-s -w" -o dist/golfg-macos-amd64 ./cmd/golfg

all: clean compile

icon:
	go install github.com/typomedia/rasterize@latest
	rasterize -i assets/img/favicon.svg --size 256
	go install github.com/typomedia/iconize@latest
	iconize favicon.png -o web/static/img/favicon.ico
