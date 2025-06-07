go build -o garus -trimpath -ldflags="-s -w -X main.appVersion=dev -X main.commit=test -X main.date=$(date +%s)" *.go
