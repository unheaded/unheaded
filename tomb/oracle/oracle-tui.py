#!/usr/bin/env python3
# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
#
# oracle-tui.py — Interactive terminal UI for the Oracle (serial console)
#
# Features:
#   - Chat interface with Oracle LLM
#   - RAG-augmented responses from the Grimoire
#   - Query history with scrollback
#   - Export conversation to markdown
#   - Works on serial console (curses-based)
#
# Usage:
#   python3 oracle-tui.py
#   python3 oracle-tui.py --batch --query-file /tmp/queries.txt
#   python3 oracle-tui.py --export-dir /opt/tomb/oracle/history

import curses
import json
import os
import re
import subprocess
import sys
import textwrap
import time
from datetime import datetime
from pathlib import Path

# --- Configuration ---
OLLAMA_URL = os.environ.get("OLLAMA_URL", "http://127.0.0.1:11434")
ORACLE_MODEL = os.environ.get("ORACLE_MODEL", "oracle-mistral")
ORACLE_ROOT = os.environ.get("ORACLE_ROOT", "/opt/tomb/oracle")
GRIMOIRE_ROOT = os.environ.get("GRIMOIRE_ROOT", "/opt/tomb/grimoire")
SYSTEM_PROMPT_FILE = os.path.join(ORACLE_ROOT, "prompts", "system-oracle.txt")
RAG_CONFIG_FILE = os.path.join(ORACLE_ROOT, "config", "rag-config.json")
HISTORY_DIR = os.path.join(ORACLE_ROOT, "history")
EXPORT_DIR = os.environ.get("EXPORT_DIR", HISTORY_DIR)
TOP_K = 5


def load_system_prompt():
    """Load the Oracle system prompt from file."""
    try:
        with open(SYSTEM_PROMPT_FILE, "r") as f:
            return f.read().strip()
    except FileNotFoundError:
        return "You are the Oracle of the Tomb of Knowledge — a security analyst for the Unheaded Kingdom."


def rag_retrieve(query, top_k=TOP_K):
    """Retrieve relevant Grimoire documents using keyword search."""
    results = []
    stop_words = {
        "the", "is", "are", "was", "were", "what", "how", "why", "when",
        "where", "who", "which", "that", "this", "with", "from", "for",
        "and", "but", "not", "can", "will", "does", "has", "have", "had",
        "a", "an", "of", "in", "to", "on", "at", "by", "do", "if", "or",
    }
    words = re.findall(r"\b[a-zA-Z0-9_]{3,}\b", query.lower())
    keywords = [w for w in words if w not in stop_words][:10]

    if not keywords:
        return ""

    search_dirs = [
        os.path.join(GRIMOIRE_ROOT, d)
        for d in ["kingdom-docs", "threat-models", "history", "mitre-attck", "nvd-cves", "cwe"]
    ]

    found = 0
    for search_dir in search_dirs:
        if not os.path.isdir(search_dir):
            continue
        for root, _dirs, files in os.walk(search_dir):
            for fname in files:
                if not fname.endswith((".md", ".txt", ".json")):
                    continue
                fpath = os.path.join(root, fname)
                try:
                    with open(fpath, "r", errors="ignore") as f:
                        content = f.read()
                    for kw in keywords:
                        if kw in content.lower():
                            # Extract context around match
                            lines = content.split("\n")
                            for i, line in enumerate(lines):
                                if kw in line.lower():
                                    start = max(0, i - 2)
                                    end = min(len(lines), i + 6)
                                    chunk = "\n".join(lines[start:end])
                                    results.append(f"--- [Source: {fpath}] ---\n{chunk}\n")
                                    found += 1
                                    break
                            if found >= top_k:
                                break
                    if found >= top_k:
                        break
                except (OSError, UnicodeDecodeError):
                    continue
            if found >= top_k:
                break
        if found >= top_k:
            break

    return "\n".join(results)


def query_ollama(prompt, system_prompt="", stream=False):
    """Query Ollama and return the response."""
    try:
        import urllib.request

        payload = {
            "model": ORACLE_MODEL,
            "prompt": prompt,
            "stream": stream,
        }
        if system_prompt:
            payload["system"] = system_prompt

        data = json.dumps(payload).encode("utf-8")
        req = urllib.request.Request(
            f"{OLLAMA_URL}/api/generate",
            data=data,
            headers={"Content-Type": "application/json"},
        )

        with urllib.request.urlopen(req, timeout=120) as resp:
            if not stream:
                body = json.loads(resp.read().decode("utf-8"))
                return body.get("response", "No response generated.")
            else:
                full_response = []
                for line in resp:
                    chunk = json.loads(line.decode("utf-8"))
                    token = chunk.get("response", "")
                    if token:
                        full_response.append(token)
                    if chunk.get("done", False):
                        break
                return "".join(full_response)
    except Exception as e:
        return f"[ERROR] Ollama query failed: {e}"


class OracleConversation:
    """Manages a conversation with history."""

    def __init__(self):
        self.messages = []  # List of (role, text, timestamp)
        self.system_prompt = load_system_prompt()

    def add_query(self, query):
        self.messages.append(("user", query, datetime.now()))

    def add_response(self, response):
        self.messages.append(("oracle", response, datetime.now()))

    def get_context_prompt(self, query, rag_context=""):
        """Build a full prompt with conversation history and RAG context."""
        parts = []

        # Include last 3 exchanges for context continuity
        recent = [m for m in self.messages if m[0] in ("user", "oracle")][-6:]
        if recent:
            parts.append("[Conversation History]")
            for role, text, ts in recent:
                label = "User" if role == "user" else "Oracle"
                parts.append(f"{label}: {text[:500]}")
            parts.append("")

        if rag_context:
            parts.append("[Grimoire Context]")
            parts.append(rag_context)
            parts.append("")

        parts.append(f"[Current Query]\n{query}")
        return "\n".join(parts)

    def export_markdown(self, filepath):
        """Export the conversation to a markdown file."""
        lines = [
            "# Oracle Conversation Log",
            f"**Date:** {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}",
            f"**Model:** {ORACLE_MODEL}",
            f"**Messages:** {len(self.messages)}",
            "",
            "---",
            "",
        ]

        for role, text, ts in self.messages:
            timestamp = ts.strftime("%H:%M:%S")
            if role == "user":
                lines.append(f"### [{timestamp}] User Query")
                lines.append("")
                lines.append(text)
                lines.append("")
            elif role == "oracle":
                lines.append(f"### [{timestamp}] Oracle Response")
                lines.append("")
                lines.append(text)
                lines.append("")
                lines.append("---")
                lines.append("")

        os.makedirs(os.path.dirname(filepath), exist_ok=True)
        with open(filepath, "w") as f:
            f.write("\n".join(lines))


class OracleTUI:
    """Curses-based TUI for the Oracle."""

    def __init__(self, stdscr):
        self.stdscr = stdscr
        self.conversation = OracleConversation()
        self.input_buffer = ""
        self.display_lines = []
        self.scroll_offset = 0
        self.query_history = []
        self.history_index = -1
        self.status_message = ""
        self.running = True

        # Curses setup
        curses.curs_set(1)
        curses.use_default_colors()
        if curses.has_colors():
            curses.init_pair(1, curses.COLOR_GREEN, -1)   # Oracle header
            curses.init_pair(2, curses.COLOR_CYAN, -1)     # User query
            curses.init_pair(3, curses.COLOR_YELLOW, -1)   # Status bar
            curses.init_pair(4, curses.COLOR_RED, -1)       # Error
            curses.init_pair(5, curses.COLOR_MAGENTA, -1)   # System info

        self.stdscr.timeout(100)  # 100ms input timeout for responsive UI

    def get_dimensions(self):
        max_y, max_x = self.stdscr.getmaxyx()
        return max_y, max_x

    def draw_header(self):
        max_y, max_x = self.get_dimensions()
        header = " TOMB OF KNOWLEDGE -- THE ORACLE "
        header = header.center(max_x - 1)
        try:
            self.stdscr.addstr(0, 0, header[:max_x - 1], curses.A_REVERSE | curses.color_pair(1))
        except curses.error:
            pass

    def draw_status_bar(self):
        max_y, max_x = self.get_dimensions()
        model_info = f" Model: {ORACLE_MODEL}"
        msg_count = f" Messages: {len(self.conversation.messages)}"
        commands = " ^X:Exit  ^E:Export  ^L:Clear  ^R:RAG  PgUp/PgDn:Scroll"

        status = f"{model_info} | {msg_count} | {commands}"
        if self.status_message:
            status = f" {self.status_message}"

        try:
            self.stdscr.addstr(
                max_y - 2, 0,
                status[:max_x - 1].ljust(max_x - 1),
                curses.A_REVERSE | curses.color_pair(3),
            )
        except curses.error:
            pass

    def draw_input_line(self):
        max_y, max_x = self.get_dimensions()
        prompt = "Oracle> "
        visible_width = max_x - len(prompt) - 1
        visible_input = self.input_buffer[-visible_width:] if len(self.input_buffer) > visible_width else self.input_buffer
        try:
            self.stdscr.addstr(max_y - 1, 0, prompt, curses.color_pair(1) | curses.A_BOLD)
            self.stdscr.addstr(max_y - 1, len(prompt), visible_input)
            self.stdscr.clrtoeol()
            cursor_x = min(len(prompt) + len(visible_input), max_x - 1)
            self.stdscr.move(max_y - 1, cursor_x)
        except curses.error:
            pass

    def draw_messages(self):
        max_y, max_x = self.get_dimensions()
        # Available display area: rows 1 through max_y-3
        display_start = 1
        display_end = max_y - 2
        display_height = display_end - display_start

        if display_height <= 0:
            return

        # Build display lines from conversation
        self.display_lines = []
        wrap_width = max_x - 4

        for role, text, ts in self.conversation.messages:
            timestamp = ts.strftime("%H:%M:%S")
            if role == "user":
                self.display_lines.append((f"[{timestamp}] You:", 2, curses.A_BOLD))
                for line in text.split("\n"):
                    wrapped = textwrap.wrap(line, wrap_width) or [""]
                    for wl in wrapped:
                        self.display_lines.append((f"  {wl}", 2, 0))
                self.display_lines.append(("", 0, 0))
            elif role == "oracle":
                self.display_lines.append((f"[{timestamp}] Oracle:", 1, curses.A_BOLD))
                for line in text.split("\n"):
                    wrapped = textwrap.wrap(line, wrap_width) or [""]
                    for wl in wrapped:
                        self.display_lines.append((f"  {wl}", 1, 0))
                self.display_lines.append(("", 0, 0))

        # Calculate visible window
        total_lines = len(self.display_lines)
        if self.scroll_offset > max(0, total_lines - display_height):
            self.scroll_offset = max(0, total_lines - display_height)

        # Auto-scroll to bottom if near the end
        if total_lines <= display_height:
            start_line = 0
        else:
            start_line = total_lines - display_height - self.scroll_offset

        start_line = max(0, start_line)

        for i in range(display_height):
            line_idx = start_line + i
            row = display_start + i
            try:
                self.stdscr.move(row, 0)
                self.stdscr.clrtoeol()
                if line_idx < len(self.display_lines):
                    text, color_pair, attrs = self.display_lines[line_idx]
                    display_text = text[:max_x - 1]
                    if color_pair > 0 and curses.has_colors():
                        self.stdscr.addstr(row, 0, display_text, curses.color_pair(color_pair) | attrs)
                    else:
                        self.stdscr.addstr(row, 0, display_text, attrs)
            except curses.error:
                pass

    def refresh_display(self):
        self.stdscr.erase()
        self.draw_header()
        self.draw_messages()
        self.draw_status_bar()
        self.draw_input_line()
        self.stdscr.refresh()

    def process_query(self, query):
        """Send query to Oracle with RAG augmentation."""
        self.conversation.add_query(query)
        self.query_history.append(query)
        self.history_index = -1

        # Show "thinking" status
        self.status_message = "Querying Grimoire and Oracle... (this may take 30-60s)"
        self.refresh_display()

        # RAG retrieval
        rag_context = rag_retrieve(query)

        # Build prompt
        prompt = self.conversation.get_context_prompt(query, rag_context)

        # Query Ollama
        response = query_ollama(prompt, self.conversation.system_prompt)

        self.conversation.add_response(response)
        self.scroll_offset = 0  # Auto-scroll to latest
        self.status_message = ""

    def export_conversation(self):
        """Export current conversation to markdown."""
        os.makedirs(EXPORT_DIR, exist_ok=True)
        filename = f"oracle-session-{datetime.now().strftime('%Y%m%d-%H%M%S')}.md"
        filepath = os.path.join(EXPORT_DIR, filename)
        self.conversation.export_markdown(filepath)
        self.status_message = f"Exported to {filepath}"

    def handle_input(self, ch):
        """Process a single keypress."""
        if ch == -1:  # Timeout, no input
            return

        # Ctrl+X — Exit
        if ch == 24:
            self.running = False
            return

        # Ctrl+E — Export
        if ch == 5:
            self.export_conversation()
            return

        # Ctrl+L — Clear display
        if ch == 12:
            self.conversation.messages.clear()
            self.display_lines.clear()
            self.scroll_offset = 0
            self.status_message = "Conversation cleared"
            return

        # Ctrl+R — Toggle RAG info
        if ch == 18:
            self.status_message = f"RAG: Grimoire={GRIMOIRE_ROOT} | top_k={TOP_K}"
            return

        # Page Up
        if ch == curses.KEY_PPAGE:
            max_y, _ = self.get_dimensions()
            self.scroll_offset = min(
                self.scroll_offset + (max_y // 2),
                max(0, len(self.display_lines) - (max_y - 3)),
            )
            return

        # Page Down
        if ch == curses.KEY_NPAGE:
            max_y, _ = self.get_dimensions()
            self.scroll_offset = max(0, self.scroll_offset - (max_y // 2))
            return

        # Up arrow — query history
        if ch == curses.KEY_UP:
            if self.query_history:
                self.history_index = min(self.history_index + 1, len(self.query_history) - 1)
                self.input_buffer = self.query_history[-(self.history_index + 1)]
            return

        # Down arrow — query history
        if ch == curses.KEY_DOWN:
            if self.history_index > 0:
                self.history_index -= 1
                self.input_buffer = self.query_history[-(self.history_index + 1)]
            elif self.history_index == 0:
                self.history_index = -1
                self.input_buffer = ""
            return

        # Enter — submit query
        if ch in (curses.KEY_ENTER, 10, 13):
            query = self.input_buffer.strip()
            self.input_buffer = ""
            if query:
                if query.lower() in ("exit", "quit", "/quit", "/exit"):
                    self.running = False
                    return
                if query.lower() in ("/export", "/save"):
                    self.export_conversation()
                    return
                if query.lower() in ("/clear", "/reset"):
                    self.conversation.messages.clear()
                    self.status_message = "Conversation cleared"
                    return
                if query.lower() == "/help":
                    help_text = (
                        "Commands: /quit /export /clear /help\n"
                        "Keys: Ctrl+X=Exit  Ctrl+E=Export  Ctrl+L=Clear  PgUp/PgDn=Scroll  Up/Down=History"
                    )
                    self.conversation.add_response(help_text)
                    return
                self.process_query(query)
            return

        # Backspace
        if ch in (curses.KEY_BACKSPACE, 127, 8):
            self.input_buffer = self.input_buffer[:-1]
            return

        # Printable character
        if 32 <= ch <= 126:
            self.input_buffer += chr(ch)
            return

    def run(self):
        """Main event loop."""
        # Welcome message
        welcome = (
            "Welcome to the Oracle of the Tomb of Knowledge.\n"
            "I am the BlackMage's analytical voice. Ask me about Kingdom security,\n"
            "threat modeling, vulnerability analysis, or attack planning.\n"
            "\n"
            "Type /help for commands. Ctrl+X or /quit to exit."
        )
        self.conversation.add_response(welcome)

        while self.running:
            self.refresh_display()
            try:
                ch = self.stdscr.getch()
                self.handle_input(ch)
            except KeyboardInterrupt:
                self.running = False

        # Offer to export on exit
        if self.conversation.messages:
            self.export_conversation()


def run_batch_mode(query_file):
    """Non-interactive batch mode for scripted queries."""
    conversation = OracleConversation()

    if not os.path.isfile(query_file):
        print(f"[ERROR] Query file not found: {query_file}", file=sys.stderr)
        sys.exit(1)

    with open(query_file, "r") as f:
        queries = [line.strip() for line in f if line.strip() and not line.startswith("#")]

    for query in queries:
        print(f"\n{'='*60}")
        print(f"Query: {query}")
        print(f"{'='*60}\n")

        rag_context = rag_retrieve(query)
        prompt = conversation.get_context_prompt(query, rag_context)
        response = query_ollama(prompt, conversation.system_prompt)

        conversation.add_query(query)
        conversation.add_response(response)

        print(response)
        print()

    # Export all results
    os.makedirs(EXPORT_DIR, exist_ok=True)
    export_path = os.path.join(EXPORT_DIR, f"batch-{datetime.now().strftime('%Y%m%d-%H%M%S')}.md")
    conversation.export_markdown(export_path)
    print(f"\n[Exported to {export_path}]")


def main():
    # Parse arguments
    batch_mode = False
    query_file = None

    args = sys.argv[1:]
    i = 0
    while i < len(args):
        if args[i] == "--batch":
            batch_mode = True
        elif args[i] == "--query-file" and i + 1 < len(args):
            query_file = args[i + 1]
            i += 1
        elif args[i] == "--export-dir" and i + 1 < len(args):
            global EXPORT_DIR
            EXPORT_DIR = args[i + 1]
            i += 1
        elif args[i] in ("--help", "-h"):
            print("Usage: oracle-tui.py [--batch --query-file FILE] [--export-dir DIR]")
            print()
            print("Interactive mode (default):")
            print("  python3 oracle-tui.py")
            print()
            print("Batch mode:")
            print("  python3 oracle-tui.py --batch --query-file /tmp/queries.txt")
            print()
            print("Environment variables:")
            print("  OLLAMA_URL       Ollama endpoint (default: http://127.0.0.1:11434)")
            print("  ORACLE_MODEL     Model name (default: oracle-mistral)")
            print("  ORACLE_ROOT      Oracle root dir (default: /opt/tomb/oracle)")
            print("  GRIMOIRE_ROOT    Grimoire root (default: /opt/tomb/grimoire)")
            sys.exit(0)
        i += 1

    if batch_mode:
        if not query_file:
            print("[ERROR] --batch requires --query-file", file=sys.stderr)
            sys.exit(1)
        run_batch_mode(query_file)
    else:
        curses.wrapper(lambda stdscr: OracleTUI(stdscr).run())


if __name__ == "__main__":
    main()
