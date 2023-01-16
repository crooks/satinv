# Build the satinv binary
FROM golang:1.19 as builder

WORKDIR /workspace

# Copy the go source
COPY go.mod go.sum satinv.go .
ADD cacher ./cacher
ADD config ./config
ADD cidrs ./cidrs
ADD multire ./multire

# Introduce the build arg check in the end of the build stage
# to avoid messing with cached layers
ARG VERSION

# Fetch modules via the proxy
ENV GOPROXY=http://plonexus01.westernpower.co.uk:8081/repository/go-proxy/
ENV GOSUMDB="sum.golang.org http://plonexus01.westernpower.co.uk:8081/repository/go-sum-proxy/"
# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -ldflags "-X main.buildVersion=${VERSION} -X main.buildDate=`date -u +%Y-%m-%d`" -o satinv .

RUN test -n "$VERSION" || (echo "VERSION not set" && false)

# Use the scratch image since we only need the go binary
FROM scratch

ARG VERSION

LABEL name=satinv \
      vendor='National Grid Electricity Distribution' \
      version=$VERSION \
      release=$VERSION \
      description='ansible inventory from Satellite' \
      summary='Create an ansible dynamic inventory from a Satellite server'

ENV USER_ID=1001

WORKDIR /
COPY --from=builder /workspace/satinv .
USER ${USER_ID}

CMD ["/satinv"]
