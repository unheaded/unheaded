#!/usr/bin/env python3
"""
17_subsample_wikipedia.py — Extract domain-relevant Wikipedia articles for Zhen FAISS.

Streams wikipedia.jsonl (29GB), filters articles by category keywords,
outputs a subsample JSONL suitable for embedding into FAISS.

Target: ~100K chunks covering:
- Geography (continents, countries, cities, regions)
- Computing (programming, algorithms, software, hardware, networking)
- Physics (mechanics, quantum, relativity, thermodynamics)
- Mathematics (algebra, calculus, geometry, number theory)
- Religion (Christianity, Islam, Buddhism, Hinduism, Judaism, etc.)
- Mythology & Folklore (Greek, Norse, Egyptian, Celtic, Japanese, etc.)
- History (wars, civilizations, empires, revolutions)
- Science (biology, chemistry, astronomy, geology)
- Philosophy (ethics, logic, epistemology, metaphysics)
- Technology (internet, protocols, cryptography, AI, Linux)

SPDX-License-Identifier: GPL-3.0-or-later
"""
import json
import sys
from pathlib import Path

WIKI_CORPUS = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'corpus' / 'wikipedia.jsonl'
OUTPUT = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'corpus' / 'wikipedia_subsample.jsonl'

# Keywords that indicate relevant articles (matched against article title)
# Broad enough to capture important articles, narrow enough to skip noise
TITLE_KEYWORDS = {
    # Geography
    'continent', 'country', 'republic', 'kingdom', 'empire', 'ocean', 'sea',
    'river', 'mountain', 'island', 'peninsula', 'desert', 'forest',
    'europe', 'asia', 'africa', 'americas', 'australia', 'antarctica',
    'arctic', 'pacific', 'atlantic', 'indian ocean', 'mediterranean',

    # Computing & Technology
    'computer', 'software', 'hardware', 'algorithm', 'programming',
    'internet', 'protocol', 'network', 'linux', 'unix', 'operating system',
    'database', 'encryption', 'cryptography', 'compiler', 'processor',
    'artificial intelligence', 'machine learning', 'neural network',
    'tcp', 'udp', 'ipv4', 'ipv6', 'http', 'dns', 'ethernet',
    'ebpf', 'kernel', 'docker', 'kubernetes', 'virtualization',
    'python', 'javascript', 'rust', 'golang',

    # Physics
    'physics', 'quantum', 'relativity', 'thermodynamics', 'mechanics',
    'electromagnetism', 'particle', 'atom', 'photon', 'electron',
    'gravity', 'energy', 'entropy', 'wave', 'radiation',
    'newton', 'einstein', 'feynman', 'dirac', 'planck', 'bohr',

    # Mathematics
    'mathematics', 'algebra', 'calculus', 'geometry', 'topology',
    'number theory', 'statistics', 'probability', 'theorem', 'equation',
    'matrix', 'vector', 'tensor', 'differential', 'integral',
    'euler', 'gauss', 'riemann', 'fourier', 'turing',

    # Religion
    'religion', 'christianity', 'islam', 'buddhism', 'hinduism',
    'judaism', 'sikhism', 'taoism', 'confucianism', 'shinto',
    'church', 'mosque', 'temple', 'bible', 'quran', 'torah',
    'prayer', 'meditation', 'scripture', 'theology',
    'jesus', 'muhammad', 'buddha', 'krishna', 'moses',
    'catholic', 'protestant', 'orthodox', 'sunni', 'shia',

    # Mythology & Folklore
    'mythology', 'myth', 'folklore', 'legend', 'fairy tale',
    'zeus', 'odin', 'thor', 'athena', 'apollo', 'isis', 'osiris',
    'greek mythology', 'norse mythology', 'egyptian mythology',
    'celtic mythology', 'japanese mythology', 'hindu mythology',
    'dragon', 'phoenix', 'unicorn', 'minotaur', 'medusa',
    'iliad', 'odyssey', 'beowulf', 'edda', 'kalevala',

    # History
    'history', 'civilization', 'revolution', 'war', 'battle',
    'ancient', 'medieval', 'renaissance', 'enlightenment',
    'roman', 'greek', 'egyptian', 'persian', 'byzantine',
    'ottoman', 'mongol', 'dynasty', 'colonialism',

    # Science
    'biology', 'chemistry', 'astronomy', 'geology', 'ecology',
    'evolution', 'genetics', 'cell', 'dna', 'protein',
    'planet', 'star', 'galaxy', 'solar system', 'universe',
    'element', 'molecule', 'periodic table',

    # Philosophy
    'philosophy', 'ethics', 'logic', 'epistemology', 'metaphysics',
    'existentialism', 'stoicism', 'utilitarianism',
    'socrates', 'plato', 'aristotle', 'kant', 'nietzsche',
    'descartes', 'hegel', 'wittgenstein',
}

# Also match articles whose titles ARE common nouns/proper nouns
# (countries, major cities, etc.)
EXACT_TITLES = {
    # Continents
    'Europe', 'Asia', 'Africa', 'North America', 'South America',
    'Australia', 'Antarctica', 'Oceania',
    # Major countries
    'United States', 'United Kingdom', 'France', 'Germany', 'Italy',
    'Spain', 'Russia', 'China', 'Japan', 'India', 'Brazil', 'Canada',
    'Mexico', 'South Korea', 'Indonesia', 'Turkey',
    'Saudi Arabia', 'Iran', 'Egypt', 'Nigeria', 'South Africa',
    'Argentina', 'Pakistan', 'Bangladesh', 'Thailand', 'Vietnam',
    'Poland', 'Ukraine', 'Romania', 'Netherlands', 'Belgium',
    'Sweden', 'Norway', 'Denmark', 'Finland', 'Ireland', 'Portugal',
    'Greece', 'Israel', 'Philippines', 'Malaysia', 'Singapore',
    'New Zealand', 'Switzerland', 'Austria', 'Czech Republic',
    # Major cities
    'London', 'Paris', 'Berlin', 'Rome', 'Madrid', 'Moscow',
    'Beijing', 'Tokyo', 'New York City', 'Los Angeles', 'Chicago',
    'Mumbai', 'Delhi', 'Shanghai', 'São Paulo', 'Cairo',
    'Istanbul', 'Lagos', 'Sydney', 'Toronto', 'Austin, Texas',
    # Key topics
    'Earth', 'Sun', 'Moon', 'Water', 'Fire', 'Light', 'Time', 'Space',
    'Life', 'Death', 'Love', 'War', 'Peace', 'Language', 'Music', 'Art',
    'Science', 'Technology', 'Engineering', 'Mathematics',
    'Democracy', 'Communism', 'Capitalism', 'Socialism', 'Fascism',
    'Human', 'Animal', 'Plant', 'Fungus', 'Virus', 'Bacteria',
    'Climate change', 'Global warming', 'Renewable energy',
    'World Wide Web', 'Email', 'Social media',
}

def title_matches(title):
    """Check if article title is relevant."""
    if title in EXACT_TITLES:
        return True
    title_lower = title.lower()
    for kw in TITLE_KEYWORDS:
        if kw in title_lower:
            return True
    return False

def main():
    if not WIKI_CORPUS.exists():
        print(f"Wikipedia corpus not found: {WIKI_CORPUS}")
        sys.exit(1)

    print(f"Scanning {WIKI_CORPUS} for relevant articles...")
    print(f"Keywords: {len(TITLE_KEYWORDS)}, Exact titles: {len(EXACT_TITLES)}")

    matched = 0
    skipped = 0
    total = 0

    with open(WIKI_CORPUS, 'r', encoding='utf-8', errors='ignore') as fin, \
         open(OUTPUT, 'w', encoding='utf-8') as fout:
        for line in fin:
            total += 1
            if total % 500000 == 0:
                print(f"  Scanned {total:,} chunks, matched {matched:,}...")

            try:
                chunk = json.loads(line)
                source = chunk.get('source', '')
                # Source format: "wikipedia/Article_Title"
                if not source.startswith('wikipedia/'):
                    skipped += 1
                    continue

                title = source[len('wikipedia/'):]
                if title_matches(title):
                    # Keep the chunk with full content
                    fout.write(line)
                    matched += 1
            except (json.JSONDecodeError, KeyError):
                skipped += 1
                continue

    print("\nDone!")
    print(f"  Total chunks scanned: {total:,}")
    print(f"  Matched chunks: {matched:,}")
    print(f"  Skipped: {skipped:,}")
    print(f"  Output: {OUTPUT} ({OUTPUT.stat().st_size / 1e6:.1f} MB)")

if __name__ == '__main__':
    main()
