FROM golang:latest AS builder
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o admission-controller .

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/admission-controller /usr/local/bin/
# Copy TLS certificate and private key
#COPY certificate.pem private-key.pem /usr/local/bin
#RUN chmod a+x admission-controller
RUN ls -l /usr/local/bin && sleep 5
#CMD ["sh", "-c", "echo running_controller! && ./admission-controller"]
