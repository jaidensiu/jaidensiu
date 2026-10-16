.PHONY: preview
preview:
	./gradlew run -q --args="."
	open -a "Google Chrome" dark_mode.svg light_mode.svg
