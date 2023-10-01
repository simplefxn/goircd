FROM golang:1.21.1-bullseye as builder

WORKDIR /app

COPY . .

RUN go mod download

RUN go mod tidy && go mod verify
RUN CGO_ENABLED=0 GOOS=linux go build -o /goircd

FROM golang:1.21.1-alpine3.18

COPY --from=builder /goircd /goircd
COPY k8s/entrypoint.sh /
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
CMD ["/goircd", "run", "--config", "/config/goircd.yaml"]