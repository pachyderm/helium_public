SHELL := /bin/bash # Use bash syntax

ENV ?= dev.env

.PHONY: docker-build

docker-build:
	docker build --progress=plain --pull --rm -f "Dockerfile" -t helium:latest .