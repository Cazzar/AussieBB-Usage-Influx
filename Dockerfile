FROM golang

WORKDIR /go/src/app
COPY . .

RUN go get -d -v
RUN go install -v

#RUN go get "github.com/ddliu/go-httpclient" "github.com/influxdata/influxdb/client/v2"

CMD ["app"]