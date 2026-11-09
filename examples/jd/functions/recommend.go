//go:build ignore

package main

type Candidate struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Sales      int64   `json:"sales"`
	Rating     float64 `json:"rating"`
	PriceMinor int64   `json:"price_minor"`
	CategoryID string  `json:"category_id"`
	Flash      bool    `json:"flash"`
}

type RankInput struct {
	Seed       string      `json:"seed"`
	CategoryID string      `json:"category_id"`
	Items      []Candidate `json:"items"`
	Limit      int64       `json:"limit"`
}

type Ranked struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Score      float64 `json:"score"`
	Sales      int64   `json:"sales"`
	Rating     float64 `json:"rating"`
	PriceMinor int64   `json:"price_minor"`
}

type RankResult struct {
	Items []Ranked `json:"items"`
	Error string   `json:"error,omitempty"`
}

// score 把销量、评分、价格与闪购加权成一个可比较的分数。
func score(c Candidate) float64 {
	value := float64(c.Sales)*3 + c.Rating*20
	if c.Flash {
		value += 40
	}
	// 高价商品略微降权，避免首页全是旗舰机。
	value -= float64(c.PriceMinor) / 100000
	return value
}

// seedBias 让同分商品的顺序随种子变化（同一种子内稳定）。
func seedBias(id, seed string) float64 {
	if seed == "" {
		return 0
	}
	sum := 0
	for i := 0; i < len(id+seed); i++ {
		sum = (sum*31 + int((id + seed)[i])) % 97
	}
	return float64(sum) / 100
}

// better 判断 a 是否应排在 b 前面。
func better(a, b Ranked) bool {
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	return a.ID < b.ID
}

// Rank 返回推荐排序结果。
// 注意：服务端 Go 解释器里给切片元素赋值（a[i] = x）会污染同一切片的其它元素，
// 所以这里只在候选下标上做选择排序，再按顺序构造新切片，不原地交换。
func Rank(in RankInput) RankResult {
	var r RankResult

	// 1) 过滤 + 去重（首页会把秒杀与热销拼在一起传入，同一商品会重复）。
	kept := make([]Ranked, 0, len(in.Items))
	for i := 0; i < len(in.Items); i++ {
		c := in.Items[i]
		if in.CategoryID != "" && c.CategoryID != in.CategoryID {
			continue
		}
		if c.ID == "" || c.Name == "" {
			continue
		}
		duplicate := false
		for j := 0; j < len(kept); j++ {
			if kept[j].ID == c.ID {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		kept = append(kept, Ranked{
			ID:         c.ID,
			Name:       c.Name,
			Score:      score(c) + seedBias(c.ID, in.Seed),
			Sales:      c.Sales,
			Rating:     c.Rating,
			PriceMinor: c.PriceMinor,
		})
	}
	if len(kept) == 0 {
		r.Error = "no candidates"
		return r
	}

	// 2) 选择排序，输出下标顺序。
	limit := in.Limit
	if limit <= 0 || limit > int64(len(kept)) {
		limit = int64(len(kept))
	}
	used := make([]bool, len(kept))
	r.Items = make([]Ranked, 0, limit)
	for round := int64(0); round < limit; round++ {
		best := -1
		for j := 0; j < len(kept); j++ {
			if used[j] {
				continue
			}
			if best < 0 || better(kept[j], kept[best]) {
				best = j
			}
		}
		if best < 0 {
			break
		}
		used[best] = true
		picked := kept[best]
		r.Items = append(r.Items, Ranked{
			ID:         picked.ID,
			Name:       picked.Name,
			Score:      picked.Score,
			Sales:      picked.Sales,
			Rating:     picked.Rating,
			PriceMinor: picked.PriceMinor,
		})
	}
	return r
}