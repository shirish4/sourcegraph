import { storiesOf } from '@storybook/react'

import { WebStory } from '../components/WebStory'

import { ExtensionBanner } from './ExtensionBanner'

const { add } = storiesOf('web/Extensions', module).addDecorator(story => <div className="p-4">{story()}</div>)

add('ExtensionBanner', () => <WebStory>{() => <ExtensionBanner />}</WebStory>, {
    design: {
        type: 'figma',
        url: 'https://www.figma.com/file/BkY8Ak997QauG0Iu2EqArv/Sourcegraph-Components?node-id=420%3A10',
    },
})
