package tools

import (
	"golang.org/x/net/html"
	"strings"
)

// findNodesByAttribute finds all nodes with a specific attribute value
func findNodesByAttribute(n *html.Node, attrName, attrValue string) []*html.Node {
	var nodes []*html.Node

	// Check if the current node has the attribute
	for _, attr := range n.Attr {
		if attr.Key == attrName && strings.Contains(attr.Val, attrValue) {
			nodes = append(nodes, n)
			break
		}
	}

	// Recursively check child nodes
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		nodes = append(nodes, findNodesByAttribute(c, attrName, attrValue)...)
	}

	return nodes
}

// findNodeByAttribute finds the first node with a specific attribute value
func findNodeByAttribute(n *html.Node, attrName, attrValue string) *html.Node {
	// Check if the current node has the attribute
	for _, attr := range n.Attr {
		if attr.Key == attrName && strings.Contains(attr.Val, attrValue) {
			return n
		}
	}

	// Recursively check child nodes
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findNodeByAttribute(c, attrName, attrValue); found != nil {
			return found
		}
	}

	return nil
}

// findAllNodes finds all nodes with a specific tag name
func findAllNodes(n *html.Node, tagName string) []*html.Node {
	var nodes []*html.Node

	// Check if the current node matches the tag name
	if n.Type == html.ElementNode && n.Data == tagName {
		nodes = append(nodes, n)
	}

	// Recursively check child nodes
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		nodes = append(nodes, findAllNodes(c, tagName)...)
	}

	return nodes
}
