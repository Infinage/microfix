# Define variables with default values. 
.PHONY: dev watch-css watch-go

# The -j2 flag tells Make to run the two dependencies in parallel.
dev:
	@echo "Starting Tailwind and Air..."
	@$(MAKE) -j2 watch-css watch-go

# Tailwind Watcher
watch-css:
	tailwindcss -i assets/css/input.css -o assets/css/index.css --minify --watch

# Go Live Reload
watch-go:
	air
