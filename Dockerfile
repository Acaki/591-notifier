FROM golang:1.17.5 as build

WORKDIR /project
COPY . .
RUN go mod download
RUN go install

FROM chromedp/headless-shell:latest
ARG cert_location=/usr/local/share/ca-certificates
RUN apt-get update && apt-get install -y ca-certificates openssl
RUN openssl s_client -showcerts -connect discord.com:443 </dev/null 2>/dev/null|openssl x509 -outform PEM > ${cert_location}/discord.crt
RUN update-ca-certificates
COPY --from=build /go/bin/591-notifier /bin/

ENTRYPOINT ["591-notifier"]
