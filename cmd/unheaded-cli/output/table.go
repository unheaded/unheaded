// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package output provides table rendering for THE GAUNTLETS CLI.
package output

import (
	"fmt"
	"io"
	"strings"
)

// Table renders data in a formatted table.
type Table struct {
	out      io.Writer
	headers  []string
	rows     [][]string
	widths   []int
	Color    bool
	sortCol  int
	sortDesc bool
}

// NewTable creates a new Table writer.
func NewTable(out io.Writer) *Table {
	return &Table{
		out:   out,
		Color: true,
	}
}

// SetColor enables or disables color output.
func (t *Table) SetColor(color bool) *Table {
	t.Color = color
	return t
}

// SetHeaders sets the table headers.
func (t *Table) SetHeaders(headers ...string) *Table {
	t.headers = headers
	t.widths = make([]int, len(headers))
	for i, h := range headers {
		t.widths[i] = len(h)
	}
	return t
}

// AddRow adds a row to the table.
func (t *Table) AddRow(cells ...string) *Table {
	row := make([]string, len(t.headers))
	for i := 0; i < len(t.headers) && i < len(cells); i++ {
		row[i] = cells[i]
		// Track max width, stripping ANSI codes for width calculation
		cellWidth := len(stripANSI(cells[i]))
		if cellWidth > t.widths[i] {
			t.widths[i] = cellWidth
		}
	}
	t.rows = append(t.rows, row)
	return t
}

// Render writes the table to the output.
func (t *Table) Render() {
	if len(t.headers) == 0 {
		return
	}

	// Calculate total width
	totalWidth := 0
	for _, w := range t.widths {
		totalWidth += w + 3 // cell + padding + separator
	}
	totalWidth++ // final separator

	// Print headers
	if t.Color {
		fmt.Fprint(t.out, ColorBold)
	}
	for i, h := range t.headers {
		fmt.Fprintf(t.out, "%-*s", t.widths[i]+2, strings.ToUpper(h))
	}
	if t.Color {
		fmt.Fprint(t.out, ColorReset)
	}
	fmt.Fprintln(t.out)

	// Print rows
	for _, row := range t.rows {
		for i, cell := range row {
			// Account for ANSI codes in padding calculation
			cellWidth := len(stripANSI(cell))
			padding := t.widths[i] - cellWidth + 2
			fmt.Fprintf(t.out, "%s%s", cell, strings.Repeat(" ", padding))
		}
		fmt.Fprintln(t.out)
	}
}

// RenderWithBorders renders the table with box borders.
func (t *Table) RenderWithBorders() {
	if len(t.headers) == 0 {
		return
	}

	// Build separator line
	separators := make([]string, len(t.widths))
	for i, w := range t.widths {
		separators[i] = strings.Repeat("-", w+2)
	}

	topBorder := "+" + strings.Join(separators, "+") + "+"
	middleBorder := "+" + strings.Join(separators, "+") + "+"
	bottomBorder := "+" + strings.Join(separators, "+") + "+"

	// Print top border
	if t.Color {
		fmt.Fprint(t.out, ColorDim)
	}
	fmt.Fprintln(t.out, topBorder)
	if t.Color {
		fmt.Fprint(t.out, ColorReset)
	}

	// Print headers
	if t.Color {
		fmt.Fprint(t.out, ColorDim+"|"+ColorReset)
	} else {
		fmt.Fprint(t.out, "|")
	}
	for i, h := range t.headers {
		if t.Color {
			fmt.Fprintf(t.out, " %s%-*s%s ", ColorBold, t.widths[i], strings.ToUpper(h), ColorReset)
			fmt.Fprint(t.out, ColorDim+"|"+ColorReset)
		} else {
			fmt.Fprintf(t.out, " %-*s |", t.widths[i], strings.ToUpper(h))
		}
	}
	fmt.Fprintln(t.out)

	// Print middle border
	if t.Color {
		fmt.Fprint(t.out, ColorDim)
	}
	fmt.Fprintln(t.out, middleBorder)
	if t.Color {
		fmt.Fprint(t.out, ColorReset)
	}

	// Print rows
	for _, row := range t.rows {
		if t.Color {
			fmt.Fprint(t.out, ColorDim+"|"+ColorReset)
		} else {
			fmt.Fprint(t.out, "|")
		}
		for i, cell := range row {
			cellWidth := len(stripANSI(cell))
			padding := t.widths[i] - cellWidth
			if t.Color {
				fmt.Fprintf(t.out, " %s%s ", cell, strings.Repeat(" ", padding))
				fmt.Fprint(t.out, ColorDim+"|"+ColorReset)
			} else {
				fmt.Fprintf(t.out, " %s%s |", cell, strings.Repeat(" ", padding))
			}
		}
		fmt.Fprintln(t.out)
	}

	// Print bottom border
	if t.Color {
		fmt.Fprint(t.out, ColorDim)
	}
	fmt.Fprintln(t.out, bottomBorder)
	if t.Color {
		fmt.Fprint(t.out, ColorReset)
	}
}

// RowCount returns the number of rows.
func (t *Table) RowCount() int {
	return len(t.rows)
}

// stripANSI removes ANSI escape codes from a string for width calculation.
func stripANSI(s string) string {
	var result strings.Builder
	inEscape := false

	for _, r := range s {
		if r == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		result.WriteRune(r)
	}

	return result.String()
}

// KeyValueTable renders key-value pairs.
type KeyValueTable struct {
	out    io.Writer
	pairs  []keyValue
	color  bool
	indent int
}

type keyValue struct {
	key   string
	value string
}

// NewKeyValueTable creates a new key-value table.
func NewKeyValueTable(out io.Writer) *KeyValueTable {
	return &KeyValueTable{
		out:   out,
		color: true,
	}
}

// SetColor enables or disables color.
func (kv *KeyValueTable) SetColor(color bool) *KeyValueTable {
	kv.color = color
	return kv
}

// SetIndent sets the indentation level.
func (kv *KeyValueTable) SetIndent(indent int) *KeyValueTable {
	kv.indent = indent
	return kv
}

// Add adds a key-value pair.
func (kv *KeyValueTable) Add(key, value string) *KeyValueTable {
	kv.pairs = append(kv.pairs, keyValue{key: key, value: value})
	return kv
}

// AddWithStatus adds a key with a status value (colorized).
func (kv *KeyValueTable) AddWithStatus(key, status string) *KeyValueTable {
	if kv.color {
		status = StatusOutput{Status: status}.Colorize(kv.color)
	}
	kv.pairs = append(kv.pairs, keyValue{key: key, value: status})
	return kv
}

// Render writes the key-value pairs.
func (kv *KeyValueTable) Render() {
	// Find max key width
	maxWidth := 0
	for _, p := range kv.pairs {
		if len(p.key) > maxWidth {
			maxWidth = len(p.key)
		}
	}

	indent := strings.Repeat(" ", kv.indent)

	for _, p := range kv.pairs {
		if kv.color {
			fmt.Fprintf(kv.out, "%s%s%-*s%s: %s\n", indent, ColorCyan, maxWidth, p.key, ColorReset, p.value)
		} else {
			fmt.Fprintf(kv.out, "%s%-*s: %s\n", indent, maxWidth, p.key, p.value)
		}
	}
}

// ListTable renders a simple list.
type ListTable struct {
	out    io.Writer
	items  []string
	bullet string
	color  bool
}

// NewListTable creates a new list table.
func NewListTable(out io.Writer) *ListTable {
	return &ListTable{
		out:    out,
		bullet: "-",
		color:  true,
	}
}

// SetColor enables or disables color.
func (lt *ListTable) SetColor(color bool) *ListTable {
	lt.color = color
	return lt
}

// SetBullet sets the bullet character.
func (lt *ListTable) SetBullet(bullet string) *ListTable {
	lt.bullet = bullet
	return lt
}

// Add adds an item to the list.
func (lt *ListTable) Add(item string) *ListTable {
	lt.items = append(lt.items, item)
	return lt
}

// Render writes the list.
func (lt *ListTable) Render() {
	for _, item := range lt.items {
		if lt.color {
			fmt.Fprintf(lt.out, "  %s%s%s %s\n", ColorCyan, lt.bullet, ColorReset, item)
		} else {
			fmt.Fprintf(lt.out, "  %s %s\n", lt.bullet, item)
		}
	}
}

// TreeNode represents a node in a tree structure.
type TreeNode struct {
	Name     string
	Value    string
	Children []*TreeNode
}

// TreeTable renders a tree structure.
type TreeTable struct {
	out   io.Writer
	root  *TreeNode
	color bool
}

// NewTreeTable creates a new tree table.
func NewTreeTable(out io.Writer, root *TreeNode) *TreeTable {
	return &TreeTable{
		out:   out,
		root:  root,
		color: true,
	}
}

// SetColor enables or disables color.
func (tt *TreeTable) SetColor(color bool) *TreeTable {
	tt.color = color
	return tt
}

// Render writes the tree.
func (tt *TreeTable) Render() {
	tt.renderNode(tt.root, "", true)
}

func (tt *TreeTable) renderNode(node *TreeNode, prefix string, isLast bool) {
	if node == nil {
		return
	}

	// Determine branch characters
	var branch, childPrefix string
	if isLast {
		branch = "`-- "
		childPrefix = "    "
	} else {
		branch = "|-- "
		childPrefix = "|   "
	}

	// Print node
	if tt.color {
		fmt.Fprintf(tt.out, "%s%s%s%s", prefix, ColorDim+branch+ColorReset, ColorBold+node.Name+ColorReset, "")
	} else {
		fmt.Fprintf(tt.out, "%s%s%s", prefix, branch, node.Name)
	}

	if node.Value != "" {
		if tt.color {
			fmt.Fprintf(tt.out, " = %s%s%s", ColorGreen, node.Value, ColorReset)
		} else {
			fmt.Fprintf(tt.out, " = %s", node.Value)
		}
	}
	fmt.Fprintln(tt.out)

	// Print children
	for i, child := range node.Children {
		isChildLast := i == len(node.Children)-1
		tt.renderNode(child, prefix+childPrefix, isChildLast)
	}
}
