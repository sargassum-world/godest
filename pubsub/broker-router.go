//nolint:funlen,gocritic,gocognit,gocyclo // All of this is copied from github.com/labstack/echo
package pubsub

// Methods

// Pub-sub broker event methods.
const (
	MethodPub     = "PUB"
	MethodSub     = "SUB"
	MethodUnsub   = "UNSUB"
	RouteNotFound = "pubsub_route_not_found"
)

type routeMethod[HandlerContext Context] struct {
	ppath   string
	pnames  []string
	handler HandlerFunc[HandlerContext]
}

type routeMethods[HandlerContext Context] struct {
	pub    *routeMethod[HandlerContext]
	sub    *routeMethod[HandlerContext]
	unsub  *routeMethod[HandlerContext]
	others map[string]*routeMethod[HandlerContext]
}

func (r *routeMethods[HandlerContext]) isHandler() bool {
	return r.pub != nil || r.sub != nil || r.unsub != nil || len(r.others) != 0
}

func (r *routeMethods[HandlerContext]) set(method string, rm *routeMethod[HandlerContext]) {
	switch method {
	case MethodPub:
		r.pub = rm
	case MethodSub:
		r.sub = rm
	case MethodUnsub:
		r.unsub = rm
	default:
		if r.others == nil {
			r.others = make(map[string]*routeMethod[HandlerContext])
		}
		if rm.handler == nil {
			delete(r.others, method)
		} else {
			r.others[method] = rm
		}
	}
}

func (r *routeMethods[HandlerContext]) get(
	method string,
) *routeMethod[HandlerContext] {
	switch method {
	case MethodPub:
		return r.pub
	case MethodSub:
		return r.sub
	case MethodUnsub:
		return r.unsub
	default:
		return r.others[method]
	}
}

// Context

// RouterContext represents the subset of the context of the current pub-sub broker event related
// to routing events to handlers.
type RouterContext[HandlerContext Context] struct {
	path    string
	pnames  []string
	pvalues []string
	handler HandlerFunc[HandlerContext]
}

// Path returns the registered path for the handler. Analogous to Echo's Context.Path.
func (c *RouterContext[HandlerContext]) Path() string {
	return c.path
}

// Param returns the path parameter value by name. Analogous to Echo's Context.Param.
func (c *RouterContext[HandlerContext]) Param(name string) string {
	// Copied from github.com/labstack/echo's context.Param method
	for i, n := range c.pnames {
		if i < len(c.pvalues) {
			if n == name {
				return c.pvalues[i]
			}
		}
	}
	return ""
}

// ParamNames returns the path parameter names. Analogous to Echo's Context.ParamNames.
func (c *RouterContext[HandlerContext]) ParamNames() []string {
	return c.pnames
}

// ParamValues returns the path parameter values. Analogous to Echo's Context.ParamValues.
func (c *RouterContext[HandlerContext]) ParamValues() []string {
	return c.pvalues[:len(c.pnames)]
}

// Node

type kind uint8

const (
	staticKind kind = iota
	paramKind
	anyKind

	paramLabel = byte(':')
	anyLabel   = byte('*')
)

type node[HandlerContext Context] struct {
	kind            kind
	label           byte
	prefix          string
	parent          *node[HandlerContext]
	staticChildren  []*node[HandlerContext]
	originalPath    string
	methods         *routeMethods[HandlerContext]
	paramChild      *node[HandlerContext]
	anyChild        *node[HandlerContext]
	paramsCount     int
	isLeaf          bool // indicates that node does not have child nodes
	isHandler       bool // indicates that node has at least one handler registered to it
	notFoundHandler *routeMethod[HandlerContext]
}

func newNode[HandlerContext Context](
	t kind, pre string, p *node[HandlerContext], sc []*node[HandlerContext], originalPath string,
	methods *routeMethods[HandlerContext], paramsCount int,
	paramChildren, anyChildren *node[HandlerContext], notFoundHandler *routeMethod[HandlerContext],
) *node[HandlerContext] {
	// Copied from github.com/labstack/echo's newNode function
	return &node[HandlerContext]{
		kind:            t,
		label:           pre[0],
		prefix:          pre,
		parent:          p,
		staticChildren:  sc,
		originalPath:    originalPath,
		methods:         methods,
		paramsCount:     paramsCount,
		paramChild:      paramChildren,
		anyChild:        anyChildren,
		isLeaf:          sc == nil && paramChildren == nil && anyChildren == nil,
		isHandler:       methods.isHandler(),
		notFoundHandler: notFoundHandler,
	}
}

func (n *node[HandlerContext]) addStaticChild(c *node[HandlerContext]) {
	// Copied from github.com/labstack/echo's node.addStaticChild method
	n.staticChildren = append(n.staticChildren, c)
}

func (n *node[HandlerContext]) findStaticChild(l byte) *node[HandlerContext] {
	// Copied from github.com/labstack/echo's node.findStaticChild method
	for _, c := range n.staticChildren {
		if c.label == l {
			return c
		}
	}
	return nil
}

func (n *node[HandlerContext]) findChildWithLabel(l byte) *node[HandlerContext] {
	// Copied from github.com/labstack/echo's node.findChildWithLabel method
	if c := n.findStaticChild(l); c != nil {
		return c
	}
	if l == paramLabel {
		return n.paramChild
	}
	if l == anyLabel {
		return n.anyChild
	}
	return nil
}

func (n *node[HandlerContext]) addMethod(method string, rm *routeMethod[HandlerContext]) {
	if method == RouteNotFound {
		n.notFoundHandler = rm
		return
	}

	n.methods.set(method, rm)
	if rm.handler != nil {
		n.isHandler = true
	} else {
		n.isHandler = n.methods.isHandler()
	}
}

func (n *node[HandlerContext]) findMethod(method string) *routeMethod[HandlerContext] {
	return n.methods.get(method)
}

func (n *node[HandlerContext]) isLeafNode() bool {
	return n.staticChildren == nil && n.paramChild == nil && n.anyChild == nil
}

// Route

// Route contains a handler and information for matching against requests. Analogous to Echo's
// Route.
type Route struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Name   string `json:"name"`
}

// Handler Router

// HandlerRouter is the registry of routes for event handler routing and topic path parameter
// parsing.
type HandlerRouter[HandlerContext Context] struct {
	tree     *node[HandlerContext]
	routes   map[string]*Route
	maxParam *int
}

// NewHandlerRouter creates a new instance of [HandlerRouter].
func NewHandlerRouter[HandlerContext Context](maxParam *int) *HandlerRouter[HandlerContext] {
	return &HandlerRouter[HandlerContext]{
		tree: &node[HandlerContext]{
			methods: new(routeMethods[HandlerContext]),
		},
		routes:   map[string]*Route{},
		maxParam: maxParam,
	}
}

// Add registers a new route for a pub-sub broker event method and topic with matching handler.
func (r *HandlerRouter[HandlerContext]) Add(method, path string, h HandlerFunc[HandlerContext]) {
	// Copied from github.com/labstack/echo's Router.Add method
	// Validate path
	if path == "" {
		path = "/"
	}
	if path[0] != '/' {
		path = "/" + path
	}
	pnames := []string{} // Param names
	ppath := path        // Pristine path

	for i, lcpIndex := 0, len(path); i < lcpIndex; i++ {
		if path[i] == ':' {
			if i > 0 && path[i-1] == '\\' {
				path = path[:i-1] + path[i:]
				i--
				lcpIndex--
				continue
			}
			j := i + 1

			r.insert(method, path[:i], staticKind, routeMethod[HandlerContext]{})
			for ; i < lcpIndex && path[i] != '/'; i++ {
			}

			pnames = append(pnames, path[j:i])
			path = path[:j] + path[i:]
			i, lcpIndex = j, len(path)

			if i == lcpIndex {
				// path node is last fragment of route path. ie. `/users/:id`
				r.insert(method, path[:i], paramKind, routeMethod[HandlerContext]{ppath, pnames, h})
			} else {
				r.insert(method, path[:i], paramKind, routeMethod[HandlerContext]{})
			}
		} else if path[i] == '*' {
			r.insert(method, path[:i], staticKind, routeMethod[HandlerContext]{})
			pnames = append(pnames, "*")
			r.insert(method, path[:i+1], anyKind, routeMethod[HandlerContext]{ppath, pnames, h})
		}
	}

	r.insert(method, path, staticKind, routeMethod[HandlerContext]{ppath, pnames, h})
}

func (r *HandlerRouter[HandlerContext]) insert(
	method, path string, t kind, rm routeMethod[HandlerContext],
) {
	// Copied from github.com/labstack/echo's Router.insert method
	// Adjust max param
	if paramLen := len(rm.pnames); *r.maxParam < paramLen {
		*r.maxParam = paramLen
	}

	currentNode := r.tree // Current node as root
	if currentNode == nil {
		panic("echo: invalid method")
	}
	search := path

	for {
		searchLen := len(search)
		prefixLen := len(currentNode.prefix)
		lcpLen := 0

		// LCP - Longest Common Prefix (https://en.wikipedia.org/wiki/LCP_array)
		max := prefixLen
		if searchLen < max {
			max = searchLen
		}
		for ; lcpLen < max && search[lcpLen] == currentNode.prefix[lcpLen]; lcpLen++ {
		}

		if lcpLen == 0 {
			// At root node
			currentNode.label = search[0]
			currentNode.prefix = search
			if rm.handler != nil {
				currentNode.kind = t
				currentNode.addMethod(method, &rm)
				currentNode.paramsCount = len(rm.pnames)
				currentNode.originalPath = rm.ppath
			}
			currentNode.isLeaf = currentNode.isLeafNode()
		} else if lcpLen < prefixLen {
			// Split node into two before we insert new node.
			// This happens when we are inserting path that is submatch of any existing inserted paths.
			// For example, we have node `/test` and now are about to insert `/te/*`. In that case
			// 1. overlapping part is `/te` that is used as parent node
			// 2. `st` is non-matching part from existing node - it gets its own node (child to `/te`)
			// 3. `/*` is the new part we are about to insert (child to `/te`)
			n := newNode(
				currentNode.kind,
				currentNode.prefix[lcpLen:],
				currentNode,
				currentNode.staticChildren,
				currentNode.originalPath,
				currentNode.methods,
				currentNode.paramsCount,
				currentNode.paramChild,
				currentNode.anyChild,
				currentNode.notFoundHandler,
			)
			// Update parent path for all children to new node
			for _, child := range currentNode.staticChildren {
				child.parent = n
			}
			if currentNode.paramChild != nil {
				currentNode.paramChild.parent = n
			}
			if currentNode.anyChild != nil {
				currentNode.anyChild.parent = n
			}

			// Reset parent node
			currentNode.kind = staticKind
			currentNode.label = currentNode.prefix[0]
			currentNode.prefix = currentNode.prefix[:lcpLen]
			currentNode.staticChildren = nil
			currentNode.originalPath = ""
			currentNode.methods = new(routeMethods[HandlerContext])
			currentNode.paramsCount = 0
			currentNode.paramChild = nil
			currentNode.anyChild = nil
			currentNode.isLeaf = false
			currentNode.isHandler = false
			currentNode.notFoundHandler = nil

			// Only Static children could reach here
			currentNode.addStaticChild(n)

			if lcpLen == searchLen {
				// At parent node
				currentNode.kind = t
				currentNode.addMethod(method, &rm)
				currentNode.paramsCount = len(rm.pnames)
				currentNode.originalPath = rm.ppath
			} else {
				// Create child node
				n = newNode(
					t, search[lcpLen:], currentNode, nil, "", new(routeMethods[HandlerContext]), 0,
					nil, nil, nil,
				)
				if rm.handler != nil {
					n.addMethod(method, &rm)
					n.paramsCount = len(rm.pnames)
					n.originalPath = rm.ppath
				}
				// Only Static children could reach here
				currentNode.addStaticChild(n)
			}
			currentNode.isLeaf = currentNode.isLeafNode()
		} else if lcpLen < searchLen {
			search = search[lcpLen:]
			c := currentNode.findChildWithLabel(search[0])
			if c != nil {
				// Go deeper
				currentNode = c
				continue
			}
			// Create child node
			n := newNode(
				t, search, currentNode, nil, rm.ppath, new(routeMethods[HandlerContext]), 0,
				nil, nil, nil,
			)
			if rm.handler != nil {
				n.addMethod(method, &rm)
				n.paramsCount = len(rm.pnames)
			}
			switch t {
			case staticKind:
				currentNode.addStaticChild(n)
			case paramKind:
				currentNode.paramChild = n
			case anyKind:
				currentNode.anyChild = n
			}
			currentNode.isLeaf = currentNode.isLeafNode()
		} else {
			// Node already exists
			if rm.handler != nil {
				currentNode.addMethod(method, &rm)
				currentNode.paramsCount = len(rm.pnames)
				currentNode.originalPath = rm.ppath
			}
		}
		return
	}
}

// Find modifies the router context with the results of attempting to match the method and path to a
// handler.
func (r *HandlerRouter[HandlerContext]) Find(
	method, path string, ctx *RouterContext[HandlerContext],
) {
	// Copied from github.com/labstack/echo's Router.Find method
	ctx.path = path
	currentNode := r.tree // Current node as root

	var (
		previousBestMatchNode *node[HandlerContext]
		matchedRouteMethod    *routeMethod[HandlerContext]
		// search stores the remaining path to check for match. By each iteration we move from start of
		// path to end of the path and search value gets shorter and shorter.
		search      = path
		searchIndex = 0
		paramIndex  int           // Param counter
		paramValues = ctx.pvalues // Use internal slice to give illusion of a dynamic slice in interface
	)

	// Backtracking is needed when a dead end (leaf node) is reached in the router tree.
	// To backtrack the current node will be changed to the parent node and the next kind for the
	// router logic will be returned based on fromKind or kind of the dead end node
	// (static > param > any).  For example if there is no static node match we should check parent
	// next sibling by kind (param).  Backtracking itself does not check if there is a next sibling,
	// this is done by the router logic.
	backtrackToNextNodeKind := func(fromKind kind) (nextNodeKind kind, valid bool) {
		previous := currentNode
		currentNode = previous.parent
		valid = currentNode != nil

		// Next node type by priority
		if previous.kind == anyKind {
			nextNodeKind = staticKind
		} else {
			nextNodeKind = previous.kind + 1
		}

		if fromKind == staticKind {
			// when backtracking is done from static kind block we did not change search so nothing to
			// restore
			return nextNodeKind, valid
		}

		// restore search to value it was before we move to current node we are backtracking from.
		if previous.kind == staticKind {
			searchIndex -= len(previous.prefix)
		} else {
			paramIndex--
			// for param/any node.prefix value is always `:` so we can not deduce searchIndex from that
			// and must use pValue for that index as it would also contain part of path we cut off before
			// moving into node we are backtracking from
			searchIndex -= len(paramValues[paramIndex])
			paramValues[paramIndex] = ""
		}
		search = path[searchIndex:]
		return nextNodeKind, valid
	}

	// Router tree is implemented by longest common prefix array (LCP array)
	// https://en.wikipedia.org/wiki/LCP_array
	// Tree search is implemented as for loop where one loop iteration is divided into 3 separate
	// blocks
	// Each of these blocks checks specific kind of node (static/param/any). Order of blocks reflex
	// their priority in routing.
	// Search order/priority is: static > param > any.
	//
	// Note: backtracking in tree is implemented by replacing/switching currentNode to previous node
	// and hoping to (goto statement) next block by priority to check if it is the match.
	for {
		prefixLen := 0 // Prefix length
		lcpLen := 0    // LCP (longest common prefix) length

		if currentNode.kind == staticKind {
			searchLen := len(search)
			prefixLen = len(currentNode.prefix)

			// LCP - Longest Common Prefix (https://en.wikipedia.org/wiki/LCP_array)
			max := prefixLen
			if searchLen < max {
				max = searchLen
			}
			for ; lcpLen < max && search[lcpLen] == currentNode.prefix[lcpLen]; lcpLen++ {
			}
		}

		if lcpLen != prefixLen {
			// No matching prefix, let's backtrack to the first possible alternative node of the decision
			// path
			nk, ok := backtrackToNextNodeKind(staticKind)
			if !ok {
				return // No other possibilities on the decision path
			} else if nk == paramKind {
				goto Param
				// NOTE: this case (backtracking from static node to previous any node) can not happen by
				// current any matching logic. Any node is end of search currently
				//} else if nk == anyKind {
				//	goto Any
			} else {
				// Not found (this should never be possible for static node we are looking currently)
				break
			}
		}

		// The full prefix has matched, remove the prefix from the remaining search
		search = search[lcpLen:]
		searchIndex = searchIndex + lcpLen

		// Finish routing if no remaining search and we are on a node with handler and matching method
		// type
		if search == "" {
			// in case of node that is handler we have exact method type match or something for 405 to use
			if currentNode.isHandler {
				// check if current node has handler registered for http method we are looking for. we store
				// currentNode as best matching in case we find no more routes matching this path+method
				if previousBestMatchNode == nil {
					previousBestMatchNode = currentNode
				}
				if rm := currentNode.findMethod(method); rm != nil {
					matchedRouteMethod = rm
					break
				}
			} else if currentNode.notFoundHandler != nil {
				matchedRouteMethod = currentNode.notFoundHandler
				break
			}
		}

		// Static node
		if search != "" {
			if child := currentNode.findStaticChild(search[0]); child != nil {
				currentNode = child
				continue
			}
		}

	Param:
		// Param node
		if child := currentNode.paramChild; search != "" && child != nil {
			currentNode = child
			i := 0
			l := len(search)
			if currentNode.isLeaf {
				// when param node does not have any children then param node should act similarly to any
				// node - consider all remaining search as match
				i = l
			} else {
				for ; i < l && search[i] != '/'; i++ {
				}
			}

			paramValues[paramIndex] = search[:i]
			paramIndex++
			search = search[i:]
			searchIndex = searchIndex + i
			continue
		}

	Any:
		// Any node
		if child := currentNode.anyChild; child != nil {
			// If any node is found, use remaining path for paramValues
			currentNode = child
			paramValues[currentNode.paramsCount-1] = search

			// update indexes/search in case we need to backtrack when no handler match is found
			paramIndex++
			searchIndex += +len(search)
			search = ""

			if rm := currentNode.findMethod(method); rm != nil {
				matchedRouteMethod = rm
				break
			}

			// we store currentNode as best matching in case we find no more routes matching this
			// path+method. Needed for 405
			if previousBestMatchNode == nil {
				previousBestMatchNode = currentNode
			}
			if currentNode.notFoundHandler != nil {
				matchedRouteMethod = currentNode.notFoundHandler
				break
			}
		}

		// Let's backtrack to the first possible alternative node of the decision path
		nk, ok := backtrackToNextNodeKind(anyKind)
		if !ok {
			break // No other possibilities on the decision path
		} else if nk == paramKind {
			goto Param
		} else if nk == anyKind {
			goto Any
		} else {
			// Not found
			break
		}
	}

	if currentNode == nil && previousBestMatchNode == nil {
		return // nothing matched at all
	}

	// matchedHandler could be method+path handler that we matched or notFoundHandler from node with
	// matching path
	// user-provided not found (404) handler has priority over generic method not found (405) handler
	// or global 404 handler
	var rPath string
	var rPNames []string
	if matchedRouteMethod != nil {
		rPath = matchedRouteMethod.ppath
		rPNames = matchedRouteMethod.pnames
		ctx.handler = matchedRouteMethod.handler
	} else {
		// use previous match as basis. although we have no matching handler we have path match.
		// so we can send http.StatusMethodNotAllowed (405) instead of http.StatusNotFound (404)
		currentNode = previousBestMatchNode

		rPath = currentNode.originalPath
		rPNames = nil // no params here
		ctx.handler = NotFoundHandler[HandlerContext]
		if currentNode.notFoundHandler != nil {
			rPath = currentNode.notFoundHandler.ppath
			rPNames = currentNode.notFoundHandler.pnames
			ctx.handler = currentNode.notFoundHandler.handler
		} else if currentNode.isHandler {
			ctx.handler = MethodNotAllowedHandler[HandlerContext]
		}
	}
	ctx.path = rPath
	ctx.pnames = rPNames
}
