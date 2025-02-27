package xpath

import (
	"errors"
	"fmt"
)

type flag int

const (
	noneFlag flag = iota
	filterFlag
)

// builder provides building an XPath expressions.
type builder struct {
	depth      int
	flag       flag
	firstInput query
}

// axisPredicate creates a predicate to predicating for this axis node.
func axisPredicate(root *axisNode) func(NodeNavigator) bool {
	// get current axix node type.
	typ := ElementNode
	switch root.AxeType {
	case "attribute":
		typ = AttributeNode
	case "self", "parent":
		typ = allNode
	default:
		switch root.Prop {
		case "comment":
			typ = CommentNode
		case "text":
			typ = TextNode
			//	case "processing-instruction":
		//	typ = ProcessingInstructionNode
		case "node":
			typ = allNode
		}
	}
	nametest := root.LocalName != "" || root.Prefix != ""
	predicate := func(n NodeNavigator) bool {
		if typ == n.NodeType() || typ == allNode || typ == TextNode {
			if nametest {
				if root.LocalName == n.LocalName() && root.Prefix == n.Prefix() {
					return true
				}
			} else {
				return true
			}
		}
		return false
	}

	return predicate
}

// processAxisNode processes a query for the XPath axis node.
func (b *builder) processAxisNode(root *axisNode) (query, error) {
	var (
		err       error
		qyInput   query
		qyOutput  query
		predicate = axisPredicate(root)
	)

	if root.Input == nil {
		qyInput = &contextQuery{}
	} else {
		if root.AxeType == "child" && (root.Input.Type() == nodeAxis) {
			if input := root.Input.(*axisNode); input.AxeType == "descendant-or-self" {
				var qyGrandInput query
				if input.Input != nil {
					qyGrandInput, _ = b.processNode(input.Input)
				} else {
					qyGrandInput = &contextQuery{}
				}
				// fix #20: https://github.com/antchfx/htmlquery/issues/20
				filter := func(n NodeNavigator) bool {
					v := predicate(n)
					switch root.Prop {
					case "text":
						v = v && n.NodeType() == TextNode
					case "comment":
						v = v && n.NodeType() == CommentNode
					}
					return v
				}
				qyOutput = &descendantQuery{Input: qyGrandInput, Predicate: filter, Self: true}
				return qyOutput, nil
			}
		}
		qyInput, err = b.processNode(root.Input)
		if err != nil {
			return nil, err
		}
	}

	switch root.AxeType {
	case "ancestor":
		qyOutput = &ancestorQuery{Input: qyInput, Predicate: predicate}
	case "ancestor-or-self":
		qyOutput = &ancestorQuery{Input: qyInput, Predicate: predicate, Self: true}
	case "attribute":
		qyOutput = &attributeQuery{Input: qyInput, Predicate: predicate}
	case "child":
		filter := func(n NodeNavigator) bool {
			v := predicate(n)
			switch root.Prop {
			case "text":
				v = v && n.NodeType() == TextNode
			case "node":
				v = v && (n.NodeType() == ElementNode || n.NodeType() == TextNode)
			case "comment":
				v = v && n.NodeType() == CommentNode
			}
			return v
		}
		qyOutput = &childQuery{Input: qyInput, Predicate: filter}
	case "descendant":
		qyOutput = &descendantQuery{Input: qyInput, Predicate: predicate}
	case "descendant-or-self":
		qyOutput = &descendantQuery{Input: qyInput, Predicate: predicate, Self: true}
	case "following":
		qyOutput = &followingQuery{Input: qyInput, Predicate: predicate}
	case "following-sibling":
		qyOutput = &followingQuery{Input: qyInput, Predicate: predicate, Sibling: true}
	case "parent":
		qyOutput = &parentQuery{Input: qyInput, Predicate: predicate}
	case "preceding":
		qyOutput = &precedingQuery{Input: qyInput, Predicate: predicate}
	case "preceding-sibling":
		qyOutput = &precedingQuery{Input: qyInput, Predicate: predicate, Sibling: true}
	case "self":
		qyOutput = &selfQuery{Input: qyInput, Predicate: predicate}
	case "namespace":
		// haha,what will you do someting??
	default:
		err = fmt.Errorf("unknown axe type: %s", root.AxeType)
		return nil, err
	}
	return qyOutput, nil
}

// processFilterNode builds query for the XPath filter predicate.
func (b *builder) processFilterNode(root *filterNode) (query, error) {
	b.flag |= filterFlag

	qyInput, err := b.processNode(root.Input)
	if err != nil {
		return nil, err
	}
	qyCond, err := b.processNode(root.Condition)
	if err != nil {
		return nil, err
	}
	qyOutput := &filterQuery{Input: qyInput, Predicate: qyCond}
	return qyOutput, nil
}

// processFunctionNode processes query for the XPath function node.
func (b *builder) processFunctionNode(root *functionNode) (query, error) {
	var qyOutput query
	switch root.FuncName {
	case "starts-with":
		arg1, err := b.processNode(root.Args[0])
		if err != nil {
			return nil, err
		}
		arg2, err := b.processNode(root.Args[1])
		if err != nil {
			return nil, err
		}
		qyOutput = &functionQuery{Input: b.firstInput, Func: startwithFunc(arg1, arg2)}
	case "ends-with":
		arg1, err := b.processNode(root.Args[0])
		if err != nil {
			return nil, err
		}
		arg2, err := b.processNode(root.Args[1])
		if err != nil {
			return nil, err
		}
		qyOutput = &functionQuery{Input: b.firstInput, Func: endwithFunc(arg1, arg2)}
	case "contains":
		arg1, err := b.processNode(root.Args[0])
		if err != nil {
			return nil, err
		}
		arg2, err := b.processNode(root.Args[1])
		if err != nil {
			return nil, err
		}
		qyOutput = &functionQuery{Input: b.firstInput, Func: containsFunc(arg1, arg2)}
	case "matches":
		//matches(string , pattern)
		if len(root.Args) != 2 {
			return nil, errors.New("xpath: matches function must have two parameters")
		}
		var (
			arg1, arg2 query
			err        error
		)
		if arg1, err = b.processNode(root.Args[0]); err != nil {
			return nil, err
		}
		if arg2, err = b.processNode(root.Args[1]); err != nil {
			return nil, err
		}
		qyOutput = &functionQuery{Input: b.firstInput, Func: matchesFunc(arg1, arg2)}
	case "substring":
		//substring( string , start [, length] )
		if len(root.Args) < 2 {
			return nil, errors.New("xpath: substring function must have at least two parameter")
		}
		var (
			arg1, arg2, arg3 query
			err              error
		)
		if arg1, err = b.processNode(root.Args[0]); err != nil {
			return nil, err
		}
		if arg2, err = b.processNode(root.Args[1]); err != nil {
			return nil, err
		}
		if len(root.Args) == 3 {
			if arg3, err = b.processNode(root.Args[2]); err != nil {
				return nil, err
			}
		}
		qyOutput = &functionQuery{Input: b.firstInput, Func: substringFunc(arg1, arg2, arg3)}
	case "substring-before", "substring-after":
		//substring-xxxx( haystack, needle )
		if len(root.Args) != 2 {
			return nil, errors.New("xpath: substring-before function must have two parameters")
		}
		var (
			arg1, arg2 query
			err        error
		)
		if arg1, err = b.processNode(root.Args[0]); err != nil {
			return nil, err
		}
		if arg2, err = b.processNode(root.Args[1]); err != nil {
			return nil, err
		}
		qyOutput = &functionQuery{
			Input: b.firstInput,
			Func:  substringIndFunc(arg1, arg2, root.FuncName == "substring-after"),
		}
	case "string-length":
		// string-length( [string] )
		if len(root.Args) < 1 {
			return nil, errors.New("xpath: string-length function must have at least one parameter")
		}
		arg1, err := b.processNode(root.Args[0])
		if err != nil {
			return nil, err
		}
		qyOutput = &functionQuery{Input: b.firstInput, Func: stringLengthFunc(arg1)}
	case "normalize-space":
		if len(root.Args) == 0 {
			return nil, errors.New("xpath: normalize-space function must have at least one parameter")
		}
		argQuery, err := b.processNode(root.Args[0])
		if err != nil {
			return nil, err
		}
		qyOutput = &functionQuery{Input: argQuery, Func: normalizespaceFunc}
	case "replace":
		//replace( string , string, string )
		if len(root.Args) != 3 {
			return nil, errors.New("xpath: replace function must have three parameters")
		}
		var (
			arg1, arg2, arg3 query
			err              error
		)
		if arg1, err = b.processNode(root.Args[0]); err != nil {
			return nil, err
		}
		if arg2, err = b.processNode(root.Args[1]); err != nil {
			return nil, err
		}
		if arg3, err = b.processNode(root.Args[2]); err != nil {
			return nil, err
		}
		qyOutput = &functionQuery{Input: b.firstInput, Func: replaceFunc(arg1, arg2, arg3)}
	case "translate":
		//translate( string , string, string )
		if len(root.Args) != 3 {
			return nil, errors.New("xpath: translate function must have three parameters")
		}
		var (
			arg1, arg2, arg3 query
			err              error
		)
		if arg1, err = b.processNode(root.Args[0]); err != nil {
			return nil, err
		}
		if arg2, err = b.processNode(root.Args[1]); err != nil {
			return nil, err
		}
		if arg3, err = b.processNode(root.Args[2]); err != nil {
			return nil, err
		}
		qyOutput = &functionQuery{Input: b.firstInput, Func: translateFunc(arg1, arg2, arg3)}
	case "not":
		if len(root.Args) == 0 {
			return nil, errors.New("xpath: not function must have at least one parameter")
		}
		argQuery, err := b.processNode(root.Args[0])
		if err != nil {
			return nil, err
		}
		qyOutput = &functionQuery{Input: argQuery, Func: notFunc}
	case "name", "local-name", "namespace-uri":
		if len(root.Args) > 1 {
			return nil, fmt.Errorf("xpath: %s function must have at most one parameter", root.FuncName)
		}
		var (
			arg query
			err error
		)
		if len(root.Args) == 1 {
			arg, err = b.processNode(root.Args[0])
			if err != nil {
				return nil, err
			}
		}
		switch root.FuncName {
		case "name":
			qyOutput = &functionQuery{Input: b.firstInput, Func: nameFunc(arg)}
		case "local-name":
			qyOutput = &functionQuery{Input: b.firstInput, Func: localNameFunc(arg)}
		case "namespace-uri":
			qyOutput = &functionQuery{Input: b.firstInput, Func: namespaceFunc(arg)}
		}
	case "true", "false":
		val := root.FuncName == "true"
		qyOutput = &functionQuery{
			Input: b.firstInput,
			Func: func(_ query, _ iterator) interface{} {
				return val
			},
		}
	case "last":
		qyOutput = &functionQuery{Input: b.firstInput, Func: lastFunc}
	case "position":
		qyOutput = &functionQuery{Input: b.firstInput, Func: positionFunc}
	case "boolean", "number", "string":
		inp := b.firstInput
		if len(root.Args) > 1 {
			return nil, fmt.Errorf("xpath: %s function must have at most one parameter", root.FuncName)
		}
		if len(root.Args) == 1 {
			argQuery, err := b.processNode(root.Args[0])
			if err != nil {
				return nil, err
			}
			inp = argQuery
		}
		f := &functionQuery{Input: inp}
		switch root.FuncName {
		case "boolean":
			f.Func = booleanFunc
		case "string":
			f.Func = stringFunc
		case "number":
			f.Func = numberFunc
		}
		qyOutput = f
	case "count":
		//if b.firstInput == nil {
		//	return nil, errors.New("xpath: expression must evaluate to node-set")
		//}
		if len(root.Args) == 0 {
			return nil, fmt.Errorf("xpath: count(node-sets) function must with have parameters node-sets")
		}
		argQuery, err := b.processNode(root.Args[0])
		if err != nil {
			return nil, err
		}
		qyOutput = &functionQuery{Input: argQuery, Func: countFunc}
	case "sum":
		if len(root.Args) == 0 {
			return nil, fmt.Errorf("xpath: sum(node-sets) function must with have parameters node-sets")
		}
		argQuery, err := b.processNode(root.Args[0])
		if err != nil {
			return nil, err
		}
		qyOutput = &functionQuery{Input: argQuery, Func: sumFunc}
	case "ceiling", "floor", "round":
		if len(root.Args) == 0 {
			return nil, fmt.Errorf("xpath: ceiling(node-sets) function must with have parameters node-sets")
		}
		argQuery, err := b.processNode(root.Args[0])
		if err != nil {
			return nil, err
		}
		f := &functionQuery{Input: argQuery}
		switch root.FuncName {
		case "ceiling":
			f.Func = ceilingFunc
		case "floor":
			f.Func = floorFunc
		case "round":
			f.Func = roundFunc
		}
		qyOutput = f
	case "concat":
		if len(root.Args) < 2 {
			return nil, fmt.Errorf("xpath: concat() must have at least two arguments")
		}
		var args []query
		for _, v := range root.Args {
			q, err := b.processNode(v)
			if err != nil {
				return nil, err
			}
			args = append(args, q)
		}
		qyOutput = &functionQuery{Input: b.firstInput, Func: concatFunc(args...)}
	case "reverse":
		if len(root.Args) == 0 {
			return nil, fmt.Errorf("xpath: reverse(node-sets) function must with have parameters node-sets")
		}
		argQuery, err := b.processNode(root.Args[0])
		if err != nil {
			return nil, err
		}
		qyOutput = &transformFunctionQuery{Input: argQuery, Func: reverseFunc}
	default:
		return nil, fmt.Errorf("not yet support this function %s()", root.FuncName)
	}
	return qyOutput, nil
}

func (b *builder) processIfNode(root *ifNode) (query, error) {
	testExpr, err := b.processNode(root.TestExpr)
	if err != nil {
		return nil, err
	}
	thenExpr, err := b.processNode(root.ThenExpr)
	if err != nil {
		return nil, err
	}
	elseExpr, err := b.processNode(root.ElseExpr)
	if err != nil {
		return nil, err
	}
	qyOutput := &ifQuery{TestExpr: testExpr, ThenExpr: thenExpr, ElseExpr: elseExpr}
	return qyOutput, nil
}

func (b *builder) processOperatorNode(root *operatorNode) (query, error) {
	left, err := b.processNode(root.Left)
	if err != nil {
		return nil, err
	}
	right, err := b.processNode(root.Right)
	if err != nil {
		return nil, err
	}
	var qyOutput query
	switch root.Op {
	case "+", "-", "*", "div", "mod": // Numeric operator
		var exprFunc func(interface{}, interface{}) interface{}
		switch root.Op {
		case "+":
			exprFunc = plusFunc
		case "-":
			exprFunc = minusFunc
		case "*":
			exprFunc = mulFunc
		case "div":
			exprFunc = divFunc
		case "mod":
			exprFunc = modFunc
		}
		qyOutput = &numericQuery{Left: left, Right: right, Do: exprFunc}
	case "=", ">", ">=", "<", "<=", "!=":
		var exprFunc func(iterator, interface{}, interface{}) interface{}
		switch root.Op {
		case "=":
			exprFunc = eqFunc
		case ">":
			exprFunc = gtFunc
		case ">=":
			exprFunc = geFunc
		case "<":
			exprFunc = ltFunc
		case "<=":
			exprFunc = leFunc
		case "!=":
			exprFunc = neFunc
		}
		qyOutput = &logicalQuery{Left: left, Right: right, Do: exprFunc}
	case "or", "and":
		isOr := false
		if root.Op == "or" {
			isOr = true
		}
		qyOutput = &booleanQuery{Left: left, Right: right, IsOr: isOr}
	case "|":
		qyOutput = &unionQuery{Left: left, Right: right}
	}
	return qyOutput, nil
}

func (b *builder) processNode(root node) (q query, err error) {
	if b.depth = b.depth + 1; b.depth > 1024 {
		err = errors.New("the xpath expressions is too complex")
		return
	}

	switch root.Type() {
	case nodeConstantOperand:
		n := root.(*operandNode)
		q = &constantQuery{Val: n.Val}
	case nodeRoot:
		q = &contextQuery{Root: true}
	case nodeAxis:
		q, err = b.processAxisNode(root.(*axisNode))
		b.firstInput = q
	case nodeFilter:
		q, err = b.processFilterNode(root.(*filterNode))
	case nodeFunction:
		q, err = b.processFunctionNode(root.(*functionNode))
	case nodeOperator:
		q, err = b.processOperatorNode(root.(*operatorNode))
	case nodeIf:
		q, err = b.processIfNode(root.(*ifNode))
	}
	return
}

// build builds a specified XPath expressions expr.
func build(expr string) (q query, err error) {
	defer func() {
		if e := recover(); e != nil {
			switch x := e.(type) {
			case string:
				err = errors.New(x)
			case error:
				err = x
			default:
				err = errors.New("unknown panic")
			}
		}
	}()
	root := parse(expr)
	b := &builder{}
	return b.processNode(root)
}
