.PHONY: preview
preview:
	go run ./cmd/profile-card assets
	open -a "Google Chrome" assets/dark_mode.svg assets/light_mode.svg
