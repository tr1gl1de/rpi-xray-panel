BINARY=rpi-panel
RPI_HOST=max@stackplayer.local
RPI_PATH=~/rpi-panel

.PHONY: build deploy test clean install-service

build:
	GOOS=linux GOARCH=arm GOARM=6 go build -o $(BINARY) ./...

run:
	go run .

test:
	go test ./...

deploy: build
	scp $(BINARY) $(RPI_HOST):$(RPI_PATH)
	ssh $(RPI_HOST) "sudo systemctl restart rpi-panel"

install-service:
	scp rpi-panel.service $(RPI_HOST):/tmp/rpi-panel.service
	ssh $(RPI_HOST) "sudo cp /tmp/rpi-panel.service /etc/systemd/system/rpi-panel.service && sudo systemctl daemon-reload && sudo systemctl enable rpi-panel"

clean:
	rm -f $(BINARY)
