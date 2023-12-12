FROM golang:latest as api-builder
WORKDIR  /app
COPY . .
RUN CGO_ENABLED=0 go build -o admission-webhook-sample .

FROM alpine:latest

# COPY application to workdir
WORKDIR /app
COPY --from=api-builder /app/admission-webhook-sample /usr/local/bin/admission-webhook-sample

RUN chmod a+x /usr/local/bin/admission-webhook-sample

# Now tell Docker what command to run when the container starts...
CMD ["/usr/local/bin/admission-webhook-sample", "-stderrthreshold=INFO","-v=3"]
