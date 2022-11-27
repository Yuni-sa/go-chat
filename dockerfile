FROM golang:alpine AS builder

WORKDIR /build

COPY . .

RUN go build

FROM alpine

WORKDIR /app

ENV PORT="8000"

COPY --from=builder /build/Chat /app/Chat

CMD ["./Chat"]