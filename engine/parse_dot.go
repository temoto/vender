package engine

import (
	"strings"

	"github.com/awalterschulze/gographviz"
	"github.com/juju/errors"
)

type dotParser struct{ *Scenario }

func NewDotParser(s *Scenario) *dotParser {
	return &dotParser{s}
}
func (self *dotParser) SetStrict(strict bool) error { return nil }
func (self *dotParser) SetDir(directed bool) error {
	if !directed {
		return errors.NotValidf("graph must be directed")
	}
	return nil
}
func (self *dotParser) SetName(name string) error {
	self.name = name
	return nil
}
func (self *dotParser) AddAttr(parentGraph string, field, value string) error {
	switch field {
	case "root":
		self.rootId = value
	}
	return nil
}
func (self *dotParser) AddPortEdge(src, srcPort, dst, dstPort string, directed bool, attrs map[string]string) error {
	// log.Printf("PortEdge src=%s:%s dst=%s:%s attr=%v", src, srcPort, dst, dstPort, attrs)
	return self.Edge(src, dst)
}
func (self *dotParser) AddEdge(src, dst string, directed bool, attrs map[string]string) error {
	return self.AddPortEdge(src, "", dst, "", directed, attrs)
}
func (self *dotParser) AddNode(parentGraph string, name string, attrs map[string]string) error {
	if self.name == parentGraph {
		parentGraph = ""
	}
	parentGraph = strings.TrimPrefix(parentGraph, "cluster_")
	// log.Printf("AddNode %s/%s", parentGraph, name)
	node := self.Get(name)
	if node == nil {
		kind := NodeInvalid
		switch attrs["shape"] {
		case "component", "point":
			kind = NodeSpecial
		case "", "cds":
			kind = NodeDoer
		}
		node = &sNode{id: name, kind: kind}
		if err := self.Add(node); err != nil {
			return err
		}
	}
	node.parent = parentGraph
	if err := self.dotParseComment(node, attrs["comment"]); err != nil {
		return errors.Annotatef(err, "node=%s", name)
	}
	return nil
}
func (self *dotParser) AddSubGraph(parentGraph string, name string, attrs map[string]string) error {
	if self.name == parentGraph {
		parentGraph = ""
	}
	name = strings.TrimPrefix(name, "cluster_")
	parentGraph = strings.TrimPrefix(parentGraph, "cluster_")
	// log.Printf("AddSubGraph %s/%s", parentGraph, name)
	node := &sNode{id: name, kind: NodeBlock, parent: parentGraph}
	return self.Add(node)
}
func (self *dotParser) String() string { return "TODO" }

func (self *dotParser) dotParseComment(node *sNode, s string) error {
	const doubleQuote = `"`
	const colon = `:`
	if s == "" {
		return nil
	}
	if strings.HasPrefix(s, doubleQuote) && strings.HasSuffix(s, doubleQuote) {
		s = s[1 : len(s)-1]
	}
	switch {
	case strings.HasPrefix(s, "/v1/"):
		// comment="/v1/v=validate-func:p=prepare-func:r=action-func"
		s = s[4:]
		parts := strings.SplitN(s, colon, 3)
		for _, p := range parts {
			switch {
			case p == "":
			case strings.HasPrefix(p, "v="):
				self.SetValidate(node.id, p[2:])
			case strings.HasPrefix(p, "se="):
				self.AddEnter(node.parent, p[3:])
			case strings.HasPrefix(p, "sl="):
				self.AddLeave(node.parent, p[3:])
			case strings.HasPrefix(p, "r="):
				self.SetRun(node.id, p[2:])
			default:
				return errors.NotValidf("comment v1 part='%s'", p)
			}
		}
		return nil
	default:
		return errors.NotValidf("comment value='%s'", s)
	}
}

func ParseDot(b []byte) (*Scenario, error) {
	ast, err := gographviz.Parse(b)
	if err != nil {
		return nil, err
	}
	s := NewScenario("")
	p := NewDotParser(s)
	err = gographviz.Analyse(ast, p)
	if err != nil {
		return nil, err
	}
	return s, nil
}
