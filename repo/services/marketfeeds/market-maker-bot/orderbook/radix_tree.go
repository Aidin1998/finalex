package orderbook

type RadixTreeNode struct {
    Children map[rune]*RadixTreeNode
    IsEnd    bool
    OrderID  string
}

type RadixTree struct {
    Root *RadixTreeNode
}

func NewRadixTree() *RadixTree {
    return &RadixTree{
        Root: &RadixTreeNode{
            Children: make(map[rune]*RadixTreeNode),
        },
    }
}

func (t *RadixTree) Insert(orderID string, key string) {
    node := t.Root
    for _, char := range key {
        if _, exists := node.Children[char]; !exists {
            node.Children[char] = &RadixTreeNode{
                Children: make(map[rune]*RadixTreeNode),
            }
        }
        node = node.Children[char]
    }
    node.IsEnd = true
    node.OrderID = orderID
}

func (t *RadixTree) Delete(key string) bool {
    return t.deleteHelper(t.Root, key, 0)
}

func (t *RadixTree) deleteHelper(node *RadixTreeNode, key string, depth int) bool {
    if node == nil {
        return false
    }
    if depth == len(key) {
        if node.IsEnd {
            node.IsEnd = false
            return len(node.Children) == 0
        }
        return false
    }
    char := rune(key[depth])
    if t.deleteHelper(node.Children[char], key, depth+1) {
        delete(node.Children, char)
        return !node.IsEnd && len(node.Children) == 0
    }
    return false
}

func (t *RadixTree) Query(key string) (string, bool) {
    node := t.Root
    for _, char := range key {
        if _, exists := node.Children[char]; !exists {
            return "", false
        }
        node = node.Children[char]
    }
    if node.IsEnd {
        return node.OrderID, true
    }
    return "", false
}