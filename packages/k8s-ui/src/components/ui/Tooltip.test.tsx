import { describe, it, expect } from 'vitest'
import { renderToString } from 'react-dom/server'
import { Tooltip } from './Tooltip'

// Pins the wrapper-className merge contract: a caller passing
// `wrapperClassName="block"` MUST override the default `inline-flex`
// on the trigger span. Without `twMerge`, plain `clsx` concatenation
// emits both `inline-flex` and `block`, and stylesheet ordering picks
// the wrong one — breaks ChartBrowser's truncation flow because the
// wrapper stays `inline-flex` and the child `truncate` never engages.
describe('Tooltip wrapper className', () => {
  it('lets the caller override the default display utility via twMerge', () => {
    const html = renderToString(
      <Tooltip content="hi" wrapperClassName="block">
        <span>child</span>
      </Tooltip>,
    )
    // Caller wins on the display group — twMerge drops the conflicting
    // default. Assert each class independently; emit order is a
    // twMerge implementation detail and varies across versions.
    expect(html).toContain('block')
    expect(html).toContain('max-w-full')
    expect(html).not.toContain('inline-flex')
  })

  it('keeps the default display utility when no caller override is supplied', () => {
    const html = renderToString(
      <Tooltip content="hi">
        <span>child</span>
      </Tooltip>,
    )
    expect(html).toContain('inline-flex max-w-full')
  })

  it('merges arbitrary non-conflicting utilities from the caller alongside the defaults', () => {
    const html = renderToString(
      <Tooltip content="hi" wrapperClassName="min-w-0 flex-1">
        <span>child</span>
      </Tooltip>,
    )
    expect(html).toContain('inline-flex')
    expect(html).toContain('max-w-full')
    expect(html).toContain('min-w-0')
    expect(html).toContain('flex-1')
  })
})
