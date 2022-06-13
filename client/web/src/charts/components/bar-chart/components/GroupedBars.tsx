import { ComponentProps, MouseEvent, ReactElement, useMemo } from 'react'

import { Group } from '@visx/group'
import { scaleBand } from '@visx/scale'
import { ScaleBand, ScaleLinear } from 'd3-scale'

import { MaybeLink } from '../../../core'
import { Category } from '../utils/get-grouped-categories'

interface GroupedBarsProps<Datum> extends ComponentProps<typeof Group> {
    categories: Category<Datum>[]
    height: number
    xScale: ScaleBand<string>
    yScale: ScaleLinear<number, number>
    getDatumName: (datum: Datum) => string
    getDatumValue: (datum: Datum) => number
    getDatumColor: (datum: Datum) => string | undefined
    getDatumLink: (datum: Datum) => string | undefined | null
    onBarHover: (datum: Datum, category: Category<Datum>) => void
    onBarLeave: () => void
    onBarClick: (datum: Datum) => void
}

export function GroupedBars<Datum>(props: GroupedBarsProps<Datum>): ReactElement {
    const {
        categories,
        height,
        xScale,
        yScale,
        getDatumName,
        getDatumValue,
        getDatumColor,
        getDatumLink,
        onBarHover,
        onBarLeave,
        onBarClick,
        ...attributes
    } = props

    const xCategoriesScale = useMemo(
        () =>
            scaleBand<string>({
                domain: [...new Set(categories.flatMap(category => category.data.map(getDatumName)))],
                range: [0, xScale.bandwidth()],
                padding: 0.2,
            }),
        [categories, xScale, getDatumName]
    )

    const handleGroupMouseMove = (event: MouseEvent): void => {
        const [category, datum] = getActiveBar({ event, xScale, xCategoriesScale, categories })

        if (category && datum) {
            onBarHover(category, datum)
        } else {
            onBarLeave()
        }
    }

    const handleGroupClick = (event: MouseEvent): void => {
        const [datum] = getActiveBar({ event, xScale, xCategoriesScale, categories })

        if (datum) {
            onBarClick(datum)
        }
    }

    return (
        <Group
            {...attributes}
            pointerEvents="bounding-box"
            onMouseMove={handleGroupMouseMove}
            onMouseLeave={onBarLeave}
            onClick={handleGroupClick}
        >
            {categories.map(category => (
                <Group key={category.id} left={xScale(category.id)} height={height}>
                    {category.data.map(datum => {
                        const isOneDatumCategory = category.data.length === 1
                        const barWidth = isOneDatumCategory ? xScale.bandwidth() : xCategoriesScale.bandwidth()
                        const barHeight = height - yScale(getDatumValue(datum))
                        const barX = isOneDatumCategory ? 0 : xCategoriesScale(getDatumName(datum))
                        const barY = yScale(getDatumValue(datum))

                        return (
                            <MaybeLink
                                key={`bar-group-bar-${category.id}-${getDatumName(datum)}`}
                                to={getDatumLink(datum)}
                            >
                                <rect
                                    x={barX}
                                    y={barY}
                                    width={barWidth}
                                    height={barHeight}
                                    fill={getDatumColor(datum)}
                                    rx={4}
                                />
                            </MaybeLink>
                        )
                    })}
                </Group>
            ))}
        </Group>
    )
}

interface GetActiveBarInput<Datum> {
    event: MouseEvent
    categories: Category<Datum>[]
    xScale: ScaleBand<string>
    xCategoriesScale: ScaleBand<string>
}

function getActiveBar<Datum>(input: GetActiveBarInput<Datum>): [datum: Datum | null, category: Category<Datum> | null] {
    const { event, xCategoriesScale, categories, xScale } = input
    const rectangle = (event.currentTarget as Element).getBoundingClientRect()
    const xCord = event.clientX - rectangle.left
    const categoryPossibleIndex = Math.floor(xCord / xScale.step())

    const category = categories[categoryPossibleIndex]

    if (!category) {
        return [null, null]
    }

    const isOneDatumCategory = category.data.length === 1

    if (isOneDatumCategory) {
        return [category.data[0], category]
    }

    const categoryWindow = categoryPossibleIndex * xScale.step()
    const possibleBarIndex = Math.floor((xCord - categoryWindow) / xCategoriesScale.step())

    if (category.data[possibleBarIndex]) {
        return [category.data[possibleBarIndex], category]
    }

    return [null, null]
}
