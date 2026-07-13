package core

import (
	"sort"

	"github.com/samber/lo"
)

// btreeDegree est le degré minimum t du B-Tree : chaque nœud (hors racine)
// porte entre t-1 et 2t-1 items, et entre t et 2t enfants.
const btreeDegree = 3

type btreeNode struct {
	items    []*indexItem
	children []*btreeNode
	leaf     bool
}

type BTree struct {
	root *btreeNode
}

// NewBTree crée un B-Tree vide (racine feuille sans item).
func NewBTree() *BTree {
	return &BTree{root: &btreeNode{leaf: true}}
}

func maxItems() int { return 2*btreeDegree - 1 }
func minItems() int { return btreeDegree - 1 }

// search localise l'item de clé de tri sk dans le sous-arbre enraciné en n.
func (n *btreeNode) search(sk string) (*indexItem, bool) {
	i := sort.Search(len(n.items), func(i int) bool { return n.items[i].sortKey >= sk })
	if i < len(n.items) && n.items[i].sortKey == sk {
		return n.items[i], true
	}
	if n.leaf {
		return nil, false
	}
	return n.children[i].search(sk)
}

// maxItem renvoie l'item le plus à droite du sous-arbre (le prédécesseur en ordre).
func (n *btreeNode) maxItem() *indexItem {
	if n.leaf {
		return n.items[len(n.items)-1]
	}
	return n.children[len(n.children)-1].maxItem()
}

// Insert ajoute key à l'ensemble des clés portant value. Si value est déjà
// indexée, on enrichit simplement son ensemble de clés (pas de restructuration).
// Sinon on insère un nouvel item, en éclatant préventivement les nœuds pleins
// rencontrés sur le chemin descendant (racine ne déborde donc jamais).
func (t *BTree) Insert(value, key string) {
	sk := sortKey(value)
	if item, found := t.root.search(sk); found {
		item.keys[key] = struct{}{}
		return
	}

	newItem := &indexItem{sortKey: sk, value: value, keys: map[string]struct{}{key: {}}}

	if len(t.root.items) == maxItems() {
		newRoot := &btreeNode{leaf: false, children: []*btreeNode{t.root}}
		splitChild(newRoot, 0)
		t.root = newRoot
	}
	insertNonFull(t.root, newItem)
}

// splitChild éclate parent.children[i] (plein, 2t-1 items) en deux nœuds de
// t-1 items, en remontant l'item médian dans parent.
func splitChild(parent *btreeNode, i int) {
	child := parent.children[i]
	mid := btreeDegree - 1
	midItem := child.items[mid]

	right := &btreeNode{leaf: child.leaf}
	right.items = append(right.items, child.items[mid+1:]...)
	if !child.leaf {
		right.children = append(right.children, child.children[mid+1:]...)
	}

	child.items = child.items[:mid]
	if !child.leaf {
		child.children = child.children[:mid+1]
	}

	parent.items = append(parent.items, nil)
	copy(parent.items[i+1:], parent.items[i:])
	parent.items[i] = midItem

	parent.children = append(parent.children, nil)
	copy(parent.children[i+2:], parent.children[i+1:])
	parent.children[i+1] = right
}

// insertNonFull insère item dans un nœud garanti non plein.
func insertNonFull(node *btreeNode, item *indexItem) {
	i := sort.Search(len(node.items), func(i int) bool { return node.items[i].sortKey >= item.sortKey })

	if node.leaf {
		node.items = append(node.items, nil)
		copy(node.items[i+1:], node.items[i:])
		node.items[i] = item
		return
	}

	if len(node.children[i].items) == maxItems() {
		splitChild(node, i)
		if item.sortKey > node.items[i].sortKey {
			i++
		}
	}
	insertNonFull(node.children[i], item)
}

// Delete retire key de l'ensemble des clés portant value. Si l'ensemble
// devient vide, l'item est retiré structurellement du B-Tree, en
// rééquilibrant (emprunt ou fusion) sur le chemin remonté.
func (t *BTree) Delete(value, key string) {
	sk := sortKey(value)
	item, found := t.root.search(sk)
	if !found {
		return
	}
	delete(item.keys, key)
	if len(item.keys) > 0 {
		return
	}

	deleteItem(t.root, sk)
	if len(t.root.items) == 0 && !t.root.leaf {
		t.root = t.root.children[0]
	}
}

// deleteItem retire structurellement l'item de clé de tri sk du sous-arbre
// enraciné en node, puis rééquilibre les enfants touchés au retour de la
// récursion (fixChild), pour ne jamais laisser un nœud sous le minimum.
func deleteItem(node *btreeNode, sk string) {
	i := sort.Search(len(node.items), func(i int) bool { return node.items[i].sortKey >= sk })
	found := i < len(node.items) && node.items[i].sortKey == sk

	if found {
		if node.leaf {
			node.items = append(node.items[:i], node.items[i+1:]...)
			return
		}
		predNode := node.children[i]
		pred := predNode.maxItem()
		node.items[i] = pred
		deleteItem(predNode, pred.sortKey)
		fixChild(node, i)
		return
	}

	if node.leaf {
		return // clé absente : rien à faire
	}
	deleteItem(node.children[i], sk)
	fixChild(node, i)
}

// fixChild rétablit l'invariant minItems sur parent.children[i] après une
// suppression dans ce sous-arbre : emprunt à un frère si possible, sinon
// fusion avec un frère.
func fixChild(parent *btreeNode, i int) {
	child := parent.children[i]
	if len(child.items) >= minItems() {
		return
	}

	// Emprunt au frère gauche (rotation droite).
	if i > 0 && len(parent.children[i-1].items) > minItems() {
		left := parent.children[i-1]
		child.items = append([]*indexItem{parent.items[i-1]}, child.items...)
		parent.items[i-1] = left.items[len(left.items)-1]
		left.items = left.items[:len(left.items)-1]
		if !left.leaf {
			moved := left.children[len(left.children)-1]
			left.children = left.children[:len(left.children)-1]
			child.children = append([]*btreeNode{moved}, child.children...)
		}
		return
	}

	// Emprunt au frère droit (rotation gauche).
	if i < len(parent.children)-1 && len(parent.children[i+1].items) > minItems() {
		right := parent.children[i+1]
		child.items = append(child.items, parent.items[i])
		parent.items[i] = right.items[0]
		right.items = right.items[1:]
		if !right.leaf {
			moved := right.children[0]
			right.children = right.children[1:]
			child.children = append(child.children, moved)
		}
		return
	}

	// Aucun emprunt possible : fusion avec un frère.
	if i < len(parent.children)-1 {
		right := parent.children[i+1]
		child.items = append(child.items, parent.items[i])
		child.items = append(child.items, right.items...)
		if !child.leaf {
			child.children = append(child.children, right.children...)
		}
		parent.items = append(parent.items[:i], parent.items[i+1:]...)
		parent.children = append(parent.children[:i+1], parent.children[i+2:]...)
	} else {
		left := parent.children[i-1]
		left.items = append(left.items, parent.items[i-1])
		left.items = append(left.items, child.items...)
		if !left.leaf {
			left.children = append(left.children, child.children...)
		}
		parent.items = append(parent.items[:i-1], parent.items[i:]...)
		parent.children = append(parent.children[:i], parent.children[i+1:]...)
	}
}

// Range parcourt le B-Tree en ordre et renvoie les clés dont la valeur
// satisfait le prédicat.
func (t *BTree) Range(op FilterOp, threshold string) []string {
	thresholdKey := sortKey(threshold)
	var result []string
	var walk func(n *btreeNode)
	walk = func(n *btreeNode) {
		for i, item := range n.items {
			if !n.leaf {
				walk(n.children[i])
			}
			if matchesRange(item.sortKey, op, thresholdKey) {
				result = append(result, lo.Keys(item.keys)...)
			}
		}
		if !n.leaf {
			walk(n.children[len(n.children)-1])
		}
	}
	walk(t.root)
	return result
}
