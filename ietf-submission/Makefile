# Unheaded Internet-Draft Build System
# Usage: make all | make clean | make draft-bellis-unheaded-protocol-foundation-00

DRAFTS = \
	draft-bellis-unheaded-protocol-foundation-00 \
	draft-bellis-unheaded-sophia-dictionary-00 \
	draft-bellis-unheaded-wotan-memory-00 \
	draft-bellis-unheaded-mbc-isa-00 \
	draft-bellis-unheaded-shim-00 \
	draft-bellis-unheaded-pqc-authentication-00

XML2RFC = xml2rfc
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

%.xml: %.md
	$(KRAMDOWN) $< > $@ 2>$(basename $<)_kramdown_err.txt

validate:
	@for d in $(DRAFTS); do \
		echo "=== Validating $$d ==="; \
		$(XML2RFC) --check $$d.xml 2>&1 | grep -E "Error:|Warning:" || echo "  Clean"; \
		echo ""; \
	done

clean:
	rm -f $(TXT_FILES) $(HTML_FILES)
	rm -f *_err.txt *_kramdown_err.txt
