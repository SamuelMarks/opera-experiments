package main

import (
	"fmt"
	"log"
	"strconv"

	"github.com/boltdb/bolt"
)

//Vertex is imaginary event block
type Vertex struct {
	Root        bool
	Clotho      bool
	Atropos     bool
	AtroposTime int64
	Timestamp   int64
	Signature   string
	PrevSelf    *Vertex
	PrevOther   *Vertex
	Frame       int
	FlagTable   map[string]int
	Hash        []byte
	RootTable   map[string]int
}

// key of finding clotho is "Frame" + "Signature" in ChkClotho

//Graph is imaginary Operachain
type Graph struct {
	Tip         *Vertex
	ChkVertex   map[string]*Vertex
	ChkClotho   map[string]*Vertex
	ClothoList  map[string]*Vertex
	AtroposList map[string]*Vertex
	TimeTable   map[string]map[string]int64
	SortList    []*Vertex
}

//NewVertex is creating vertex
func NewVertex() *Vertex {
	newVertex := Vertex{FlagTable: make(map[string]int)}

	return &newVertex
}

//NewGraph is creating graph
func (oc *Operachain) NewGraph() *Graph {
	newGraph := Graph{
		nil,
		make(map[string]*Vertex),
		make(map[string]*Vertex),
		make(map[string]*Vertex),
		make(map[string]*Vertex),
		make(map[string]map[string]int64),
		[]*Vertex{},
	}

	var tip []byte

	err := oc.Db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(blocksBucket))
		tip = b.Get([]byte("l"))

		return nil
	})

	if err != nil {
	}

	newGraph.Tip = oc.BuildGraph(tip, newGraph.ChkVertex, &newGraph)

	return &newGraph
}

//BuildGraph initialize graph based on DB
func (oc *Operachain) BuildGraph(hash []byte, rV map[string]*Vertex, g *Graph) *Vertex {
	newVertex, exists := rV[string(hash)]
	if exists {
		return newVertex
	}
	newVertex = NewVertex()
	var prevSelf, prevOther []byte

	var block *Block

	err := oc.Db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(blocksBucket))
		encodedBlock := b.Get(hash)
		block = DeserializeBlock(encodedBlock)

		return nil
	})

	if err != nil {
		log.Panic(err)
	}

	newVertex.Signature = block.Signature
	prevSelf = block.PrevSelfHash
	prevOther = block.PrevOtherHash
	newVertex.Hash = block.Hash
	rV[string(hash)] = newVertex
	newVertex.Timestamp = block.Timestamp

	if prevSelf != nil {
		selfVertex := oc.BuildGraph(prevSelf, rV, g)
		newVertex.PrevSelf = selfVertex
	}
	if prevOther != nil {
		otherVertex := oc.BuildGraph(prevOther, rV, g)
		newVertex.PrevOther = otherVertex
	}

	// Complete searching ancestor blocks of newVertex

	if newVertex.PrevSelf != nil {
		if newVertex.PrevSelf.Frame == newVertex.PrevOther.Frame {
			newVertex.FlagTable = Merge(newVertex.PrevSelf.FlagTable, newVertex.PrevOther.FlagTable, newVertex.PrevSelf.Frame)
			if len(newVertex.FlagTable) >= supraMajor {
				newVertex.Root = true
				newVertex.Frame = newVertex.PrevSelf.Frame + 1
				newVertex.RootTable = Copy(newVertex.FlagTable)
				newVertex.FlagTable = make(map[string]int)
				newVertex.FlagTable[newVertex.Signature] = newVertex.Frame

				g.ChkClotho[strconv.Itoa(newVertex.Frame)+"_"+newVertex.Signature] = newVertex
				// Clotho check

				g.ClothoChecking(newVertex)
				g.AtroposTimeSelection(newVertex)
			} else {
				newVertex.Root = false
				newVertex.Frame = newVertex.PrevSelf.Frame
			}
		} else if newVertex.PrevSelf.Frame > newVertex.PrevOther.Frame {
			newVertex.Root = false
			newVertex.Frame = newVertex.PrevSelf.Frame
			newVertex.FlagTable = Copy(newVertex.PrevSelf.FlagTable)
		} else {
			newVertex.Root = true
			newVertex.Frame = newVertex.PrevOther.Frame
			otherRoot := g.ChkClotho[strconv.Itoa(newVertex.PrevOther.Frame)+"_"+newVertex.PrevOther.Signature]
			newVertex.RootTable = Merge(newVertex.PrevSelf.FlagTable, otherRoot.RootTable, newVertex.Frame-1)
			newVertex.FlagTable = Copy(newVertex.PrevOther.FlagTable)
			newVertex.FlagTable[newVertex.Signature] = newVertex.Frame

			g.ChkClotho[strconv.Itoa(newVertex.Frame)+"_"+newVertex.Signature] = newVertex
			// Clotho check
			g.ClothoChecking(newVertex)

			g.AtroposTimeSelection(newVertex)
		}
	} else {
		newVertex.Root = true
		newVertex.Frame = 0
		newVertex.FlagTable[newVertex.Signature] = newVertex.Frame
		g.ClothoList[strconv.Itoa(newVertex.Frame)+"_"+newVertex.Signature] = newVertex
	}

	return newVertex
}

// ClothoChecking checks whether ancestor of the vertex is colotho
func (g *Graph) ClothoChecking(v *Vertex) {
	// Clotho check
	ccList := make(map[string]map[string]bool)

	for key, val := range v.RootTable {
		prevRoot, exists := g.ChkClotho[strconv.Itoa(val)+"_"+key]
		if !exists {
			continue
		}

		for rkey, rval := range prevRoot.RootTable {
			prevPrevRoot, exists := g.ChkClotho[strconv.Itoa(rval)+"_"+rkey]
			if !exists {
				continue
			}

			for rrkey, rrval := range prevPrevRoot.RootTable {
				_, exists := g.ChkClotho[strconv.Itoa(rrval)+"_"+rrkey]
				if !exists {
					continue
				}
				if ccList[strconv.Itoa(rrval)+"_"+rrkey] == nil {
					ccList[strconv.Itoa(rrval)+"_"+rrkey] = make(map[string]bool)
				}

				_, exists2 := ccList[strconv.Itoa(rrval)+"_"+rrkey][strconv.Itoa(rval)+"_"+rkey]
				if !exists2 {
					ccList[strconv.Itoa(rrval)+"_"+rrkey][strconv.Itoa(rval)+"_"+rkey] = true
				}

			}
		}
	}

	for key, val := range ccList {
		if len(val) >= subMajor {
			prevRoot := g.ChkClotho[key]
			g.ClothoList[key] = prevRoot
			prevRoot.Clotho = true
			if g.TimeTable[strconv.Itoa(v.Frame)+"_"+v.Signature] == nil {
				g.TimeTable[strconv.Itoa(v.Frame)+"_"+v.Signature] = make(map[string]int64)
			}
			g.TimeTable[strconv.Itoa(v.Frame)+"_"+v.Signature][strconv.Itoa(prevRoot.Frame)+"_"+prevRoot.Signature] = v.Timestamp
			fmt.Printf("%s is assigned as Clotho by %s\n", key, strconv.Itoa(v.Frame)+"_"+v.Signature)
		}
	}
}

// AtroposTimeSelection selects time from set of previous vertex
func (g *Graph) AtroposTimeSelection(v *Vertex) {
	countMap := make(map[string]map[int64]int)

	for prevKey, prevVal := range v.RootTable {
		prevSig := strconv.Itoa(prevVal) + "_" + prevKey
		for key, val := range g.TimeTable[prevSig] {

			if countMap[key] == nil {
				countMap[key] = make(map[int64]int)
			}
			//fmt.Println("key")
			cval, exists2 := countMap[key][val]
			if exists2 {
				countMap[key][val] = cval + 1
			} else {
				countMap[key][val] = 1
			}
		}
	}

	for key, val := range countMap {
		maxVal := 0
		var maxInd int64

		clotho := g.ClothoList[key]
		if (v.Frame-clotho.Frame)%4 == 0 {
			for time, count := range val {
				if maxVal == 0 {
					maxVal = count
					maxInd = time
				} else if time < maxInd {
					maxInd = time
				}
			}

			g.TimeTable[strconv.Itoa(v.Frame)+"_"+v.Signature][key] = maxInd
		} else {
			for time, count := range val {
				if count > maxVal {
					maxVal = count
					maxInd = time
				} else if count == maxVal && time < maxInd {
					maxInd = time
				}
			}

			if maxVal >= supraMajor {
				fmt.Println("atropos", clotho.Signature, clotho.Frame, maxInd)
				clotho.Atropos = true
				clotho.AtroposTime = maxInd
				g.AssignAtroposTime(clotho)
			} else {
				g.TimeTable[strconv.Itoa(v.Frame)+"_"+v.Signature][key] = maxInd
			}
		}
	}
}

// AssignAtroposTime is
func (g *Graph) AssignAtroposTime(atropos *Vertex) {
	batchList := []*Vertex{}
	sortList := []*Vertex{}
	aTime := atropos.AtroposTime

	batchList = append(batchList, atropos)
	for {
		if len(batchList) == 0 {
			break
		}

		currentVertex := batchList[0]
		batchList = batchList[1:]
		//fmt.Println(1, sortList)

		sortList = append([]*Vertex{currentVertex}, sortList...)
		//fmt.Println(2, sortList)
		chk := false
		if currentVertex.AtroposTime == 0 || aTime < currentVertex.AtroposTime {
			currentVertex.AtroposTime = aTime
			chk = true
		}

		if chk {
			if currentVertex.PrevSelf != nil {
				batchList = append(batchList, currentVertex.PrevSelf)
			}
			if currentVertex.PrevOther != nil {
				batchList = append(batchList, currentVertex.PrevOther)
			}
		}
	}
	//fmt.Println(sortList)
	//Sort vertex
	for {
		if len(sortList) == 0 {
			break
		}

		currentVertex := sortList[0]
		sortList = sortList[1:]

		index := len(g.SortList) - 1

		for {
			if index < 0 {
				break
			}
			compVertex := g.SortList[index]
			if compVertex.AtroposTime < currentVertex.AtroposTime {
				break
			} else if compVertex.AtroposTime == currentVertex.AtroposTime {
				if compVertex.Timestamp < currentVertex.Timestamp {
					break
				} else if compVertex.Timestamp == currentVertex.Timestamp {
					chk := false
					for idn, val := range currentVertex.Hash {
						if val > compVertex.Hash[idn] {
							chk = true
							break
						} else if val < compVertex.Hash[idn] {
							chk = false
							break
						}
					}

					if chk {
						break
					}
				}
			}

			index--
		}
		g.Insert(index+1, currentVertex)
	}
}

// Insert item into list
func (g *Graph) Insert(i int, v *Vertex) {
	if i == len(g.SortList) {
		g.SortList = append(g.SortList, v)

		return
	}
	g.SortList = append(g.SortList, nil)
	copy(g.SortList[i+1:], g.SortList[i:])
	g.SortList[i] = v

}

// Merge is union between parent flagtable
func Merge(sv, ov map[string]int, fNum int) map[string]int {
	ret := make(map[string]int)
	for sKey, sVal := range sv {
		if sVal == fNum {
			ret[sKey] = sVal
		}
	}

	for oKey, oVal := range ov {
		_, exists := ret[oKey]
		if !exists {
			if oVal == fNum {
				ret[oKey] = oVal
			}
		}
	}

	return ret
}

// Copy copies flagtable into roottalbe
func Copy(c map[string]int) map[string]int {
	ret := make(map[string]int)
	for key, val := range c {
		ret[key] = val
	}

	return ret
}

// Max selects maximum value between parent frame numbers
func Max(sf, of int) int {
	if sf > of {
		return sf
	}
	return of
}
