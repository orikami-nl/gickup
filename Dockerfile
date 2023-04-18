FROM golang:1.17-alpine as builder

# Install dependencies for copy
RUN apk add -U --no-cache ca-certificates tzdata git

# Use an valid GOPATH and copy the files
WORKDIR /go/src/github.com/cooperspencer/gickup
COPY go.mod .
COPY go.sum .
RUN go mod tidy
COPY . .

# Fetching dependencies and build the app
RUN go get -d -v ./...
RUN CGO_ENABLED=0 go build -a -installsuffix cgo -o app .

# Use scratch as production environment -> Small builds
FROM ubuntu:22.04 as production
WORKDIR /
# Copy valid SSL certs from the builder for fetching github/gitlab/...
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
# Copy zoneinfo for getting the right cron timezone
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
# Copy the main executable from the builder
COPY --from=builder /go/src/github.com/cooperspencer/gickup/app /gickup/app

# Installing gcsfuse
RUN apt-get update && apt-get install -y curl fuse && curl -LO https://github.com/GoogleCloudPlatform/gcsfuse/releases/download/v0.42.3/gcsfuse_0.42.3_amd64.deb && dpkg -i gcsfuse_0.42.3_amd64.deb 

ENTRYPOINT [ "/gickup/app" ]
CMD [ "/gickup/conf.yml" ]
