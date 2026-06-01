# Define variables with default values. 
.PHONY: dev watch-css watch-templ

# The -j2 flag tells Make to run the two dependencies in parallel.
dev:
	@$(MAKE) -j2 watch-css watch-templ

# Tailwind Watcher
watch-css:
	tailwindcss -i assets/css/input.css -o assets/css/index.css --watch

# Templ Watcher + Go Server Runner + Live Reload Proxy
watch-templ:
	templ generate --watch --proxy="http://localhost:3000" --cmd="go run ."
