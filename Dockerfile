FROM golang:1.22 as build

WORKDIR /app

COPY . .

RUN go mod download && \
    make build

FROM gcr.io/distroless/static-debian11

COPY --from=build /app/bin/api /

CMD ["/api"]