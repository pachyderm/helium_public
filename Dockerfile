FROM debian:bullseye-slim AS base

RUN apt update -y && apt install -y \
    ca-certificates \
    git \
    jq \
    wget \
    curl \
    python \
    unzip \
  && rm -rf /var/lib/apt/lists/*

RUN wget https://get.pulumi.com/releases/sdk/pulumi-v3.31.0-linux-x64.tar.gz
RUN tar xvf pulumi-v3.31.0-linux-x64.tar.gz

RUN curl https://dl.google.com/dl/cloudsdk/release/google-cloud-sdk.tar.gz > /tmp/google-cloud-sdk.tar.gz
RUN mkdir -p /usr/local/gcloud \
  && tar -C /usr/local/gcloud -xvf /tmp/google-cloud-sdk.tar.gz \
  && /usr/local/gcloud/google-cloud-sdk/install.sh

RUN curl -fsSL https://deb.nodesource.com/setup_current.x | bash - && \
  apt-get install -y nodejs \
  build-essential && \
  node --version && \ 
  npm --version

RUN curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "awscliv2.zip"
RUN unzip awscliv2.zip
RUN ./aws/install

RUN curl --silent --location "https://github.com/weaveworks/eksctl/releases/latest/download/eksctl_$(uname -s)_amd64.tar.gz" | tar xz -C /tmp
RUN mv /tmp/eksctl /usr/local/bin

RUN curl -o aws-iam-authenticator https://s3.us-west-2.amazonaws.com/amazon-eks/1.21.2/2021-07-05/bin/linux/amd64/aws-iam-authenticator
RUN chmod +x ./aws-iam-authenticator

RUN mkdir -p $HOME/bin && cp ./aws-iam-authenticator $HOME/bin/aws-iam-authenticator && export PATH=$PATH:$HOME/bin
RUN echo 'export PATH=$PATH:$HOME/bin' >> ~/.bashrc

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
