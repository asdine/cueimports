package file

wall: {
	color: colors.#Red
	size:  dimensions.#Dimensions & {
		w: 100
		h: 100
	}
	altSize: alt.#AltDimensions & {
		w: 150
		h: 150
	}
	priceM2: math.Floor(size.w * size.h / 10000)
	objects: [
		shapes.#Square & {
			d: {
				w: 100
				h: 100
			}
		},
	]
}

floor: {
	color: colors.#Color
}

enc: json.Marshal(wall)

ceiling: {
	color: default.color
}
