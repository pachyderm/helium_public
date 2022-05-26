FROM debian:bullseye-slim AS base

RUN apt update -y && apt install -y \
    ca-certificates \
    git \
    jq \
    wget \
    curl \
    python \
    && rm -rf /var/lib/apt/lists/*

RUN wget https://get.pulumi.com/releases/sdk/pulumi-v3.31.0-linux-x64.tar.gz
RUN tar xvf pulumi-v3.31.0-linux-x64.tar.gz

RUN curl https://dl.google.com/dl/cloudsdk/release/google-cloud-sdk.tar.gz > /tmp/google-cloud-sdk.tar.gz
RUN mkdir -p /usr/local/gcloud \
  && tar -C /usr/local/gcloud -xvf /tmp/google-cloud-sdk.tar.gz \
  && /usr/local/gcloud/google-cloud-sdk/install.sh

FROM golang:1.17-alpine AS build

WORKDIR /app

RUN apk add --no-cache curl ca-certificates

COPY go.mod go.sum ./

RUN go mod download

COPY ./ ./

RUN CGO_ENABLED=0 go build -o out main.go

FROM base

ARG HELIUM_CLIENT_SECRET
ARG HELIUM_CLIENT_ID
ARG PULUMI_ACCESS_TOKEN

COPY --from=build /app/out /out
COPY --from=build /app/root.py /root.py
COPY --from=build /app/templates /templates

ENV PATH $PATH:/usr/local/gcloud/google-cloud-sdk/bin
ENV PATH "${HOME}/pulumi:$PATH"
ENV HELIUM_MODE "API"

ENV HELIUM_CLIENT_ID $HELIUM_CLIENT_ID
ENV HELIUM_CLIENT_SECRET $HELIUM_CLIENT_SECRET
ENV PULUMI_ACCESS_TOKEN $PULUMI_ACCESS_TOKEN

CMD ["/out"]

EXPOSE 2323
