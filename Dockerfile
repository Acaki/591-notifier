FROM golang:1.17.5 as build

WORKDIR /project
COPY . .
RUN go mod download
RUN go install

FROM chromedp/headless-shell:latest
COPY --from=build /go/bin/591-notifier /bin/
ENTRYPOINT ["591-notifier"]
