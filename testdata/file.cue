package file

wall: {
	color: colors.#Red
	size:  dimensions.#Dimensions & {
		w: 100
		h: 100
	}
	priceM2: math.Floor(w * h / 10000)
}

floor: {
	color: colors.#Color
}

enc: json.Marshal(wall)

ceiling: {
	color: default.color
}
