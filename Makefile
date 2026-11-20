SCRIPTS := bin/q hooks/notes-hook.sh statusline/statusline.sh install.sh uninstall.sh

.PHONY: lint
lint:
	@for f in $(SCRIPTS); do bash -n "$$f" || exit 1; done
	shellcheck -x $(SCRIPTS)
