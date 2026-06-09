package legacy

// LowerProficiency mirrors command10.c lower_prof. It consumes an experience
// loss across five weapon proficiency slots and four magic realm slots.
func LowerProficiency(proficiency [5]int, realm [4]int, exp int) ([5]int, [4]int) {
	var prof [5]int64
	var realms [4]int64
	var total int64
	for i, value := range proficiency {
		prof[i] = int64(value)
		total += prof[i]
	}
	for i, value := range realm {
		realms[i] = int64(value)
		total += realms[i]
	}

	profLoss := int64(exp)
	if profLoss < 0 {
		profLoss = 0
	}
	if profLoss > total {
		profLoss = total
	}

	below := 0
	for profLoss > 9 && below < 9 {
		below = 0
		for n := 0; n < 9; n++ {
			part := profLoss / int64(9-n)
			profLoss -= part
			if n < 5 {
				prof[n] -= part
				if prof[n] < 0 {
					below++
					profLoss -= prof[n]
					prof[n] = 0
				}
				continue
			}

			idx := n - 5
			realms[idx] -= part
			if realms[idx] < 0 {
				below++
				profLoss -= realms[idx]
				realms[idx] = 0
			}
		}
	}

	best := 0
	for i := 1; i < len(prof); i++ {
		if prof[i] > prof[best] {
			best = i
		}
	}
	if prof[best] < 1024 {
		prof[best] = 1024
	}

	for i := range proficiency {
		proficiency[i] = int(prof[i])
	}
	for i := range realm {
		realm[i] = int(realms[i])
	}
	return proficiency, realm
}
