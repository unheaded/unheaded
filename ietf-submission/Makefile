# Unheaded Internet-Draft Build System
# Usage: make all | make clean | make draft-bellis-unheaded-protocol-foundation-00

DRAFTS = \
	draft-bellis-unheaded-protocol-foundation-00 \
	draft-bellis-unheaded-sophia-dictionary-00 \
	draft-bellis-unheaded-wotan-memory-00 \
	draft-bellis-unheaded-mbc-isa-00 \
	draft-bellis-unheaded-shim-00 \
	draft-bellis-unheaded-pqc-authentication-00

# xml2rfc 3.x; on west it lives in ~/.local/bin.
XML2RFC ?= $(or $(shell command -v xml2rfc 2>/dev/null),$(HOME)/.local/bin/xml2rfc)
KRAMDOWN = $(HOME)/.local/share/gem/ruby/3.3.0/gems/kramdown-rfc2629-1.7.34/bin/kramdown-rfc2629

TXT_FILES = $(addsuffix .txt,$(DRAFTS))
HTML_FILES = $(addsuffix .html,$(DRAFTS))

.PHONY: all clean validate txt html

all: txt html
	@echo ""
	@echo "=== Build complete ==="
	@for d in $(DRAFTS); do \
		printf "  %-55s %s  %s\n" "$$d" \
			"$$([ -f $$d.txt ] && echo 'txt:OK' || echo 'txt:FAIL')" \
			"$$([ -f $$d.html ] && echo 'html:OK' || echo 'html:FAIL')"; \
	done

txt: $(TXT_FILES)
html: $(HTML_FILES)

%.txt: %.xml
	$(XML2RFC) --text $<

%.html: %.xml
	$(XML2RFC) --html $<

# The .xml files are the submission sources and are maintained by hand (they
# carry fixes the .md twins do not). Never let make regenerate them from the
# .md: a newer .md mtime would silently overwrite the reviewed XML. kramdown
# output goes beside them for comparison only: make <draft>.kramdown.xml
%.kramdown.xml: %.md
	$(KRAMDOWN) $< > $@ 2>$(basename $<)_kramdown_err.txt

# Renders each draft to text (discarded) and fails on any xml2rfc error.
# xml2rfc 3.x has no --check flag: the old target passed it, got "unrecognized
# arguments" (no "Error:" in that message) and printed Clean for every draft
# without validating anything. Warnings are counted, not fatal.
validate:
	@fail=0; for d in $(DRAFTS); do \
		echo "=== Validating $$d ==="; \
		out=$$($(XML2RFC) --text -o /dev/null $$d.xml 2>&1); rc=$$?; \
		w=$$(printf '%s\n' "$$out" | grep -c 'Warning:'); \
		if [ $$rc -ne 0 ] || printf '%s\n' "$$out" | grep -q 'Error:'; then \
			printf '%s\n' "$$out" | grep -A3 'Error:'; echo "  FAIL (exit $$rc)"; fail=1; \
		else echo "  Clean ($$w warnings)"; fi; \
		echo ""; \
	done; exit $$fail

clean:
	rm -f $(TXT_FILES) $(HTML_FILES)
	rm -f *_err.txt *_kramdown_err.txt *.kramdown.xml
