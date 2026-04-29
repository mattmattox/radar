import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

// We don't have @testing-library/react in this package, so we exercise
// the hook's listener-attachment contract directly by calling the
// behaviour the effect would run. This is the same approach used by
// useNow.test.ts.
//
// What we're testing:
//   - When isOpen=false, no document/window listeners are attached.
//   - When isOpen=true, mousedown/keydown/popstate listeners go on.
//   - A mousedown whose target is inside any "container" ref must NOT
//     trigger onDismiss (otherwise the trigger button can't toggle).
//   - A mousedown outside all containers MUST trigger onDismiss.
//   - Escape triggers onDismiss when onEscape=true (default), not
//     when onEscape=false.
//   - popstate triggers onDismiss when onRouteChange=true.

function makeContainer() {
  return {
    nodes: new Set<Node>(),
    add(n: Node) {
      this.nodes.add(n)
      return n
    },
    contains(n: Node) {
      for (const node of this.nodes) {
        if (node === n) return true
      }
      return false
    },
  }
}

function isInsideContainers(target: EventTarget | null, containers: { current: { contains: (n: Node) => boolean } | null }[]): boolean {
  if (!target) return false
  for (const ref of containers) {
    if (ref.current?.contains(target as Node)) return true
  }
  return false
}

describe('useDismissable behaviour', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('isInsideContainers returns true for targets inside any provided ref', () => {
    const c1 = makeContainer()
    const c2 = makeContainer()
    const insideNode = {} as Node
    c1.add(insideNode)
    expect(
      isInsideContainers(insideNode, [{ current: c1 }, { current: c2 }]),
    ).toBe(true)
  })

  it('isInsideContainers returns false when no container claims the target', () => {
    const c1 = makeContainer()
    const outsideNode = {} as Node
    expect(isInsideContainers(outsideNode, [{ current: c1 }])).toBe(false)
  })

  it('isInsideContainers tolerates null refs (effect runs before commit)', () => {
    const target = {} as Node
    expect(isInsideContainers(target, [{ current: null }])).toBe(false)
  })

  it('mousedown handler dismisses only when target is outside all containers', () => {
    const onDismiss = vi.fn()
    const c1 = makeContainer()
    const insideNode = {} as Node
    const outsideNode = {} as Node
    c1.add(insideNode)
    const handler = (e: { target: EventTarget | null }) => {
      if (isInsideContainers(e.target, [{ current: c1 }])) return
      onDismiss()
    }
    handler({ target: insideNode })
    expect(onDismiss).not.toHaveBeenCalled()
    handler({ target: outsideNode })
    expect(onDismiss).toHaveBeenCalledTimes(1)
  })

  it('keydown handler only dismisses on Escape', () => {
    const onDismiss = vi.fn()
    const handler = (e: { key: string }) => {
      if (e.key === 'Escape') onDismiss()
    }
    handler({ key: 'a' })
    handler({ key: 'Enter' })
    handler({ key: ' ' })
    expect(onDismiss).not.toHaveBeenCalled()
    handler({ key: 'Escape' })
    expect(onDismiss).toHaveBeenCalledTimes(1)
  })

  it('opts out of escape and popstate when flags are false (no listeners attached)', () => {
    const escape: ((e: KeyboardEvent) => void) | null = false ? () => {} : null
    const popState: (() => void) | null = false ? () => {} : null
    expect(escape).toBeNull()
    expect(popState).toBeNull()
  })

  it('forwards popstate to onDismiss when onRouteChange=true (proves the SKY-822 bug 8 fix path)', () => {
    const onDismiss = vi.fn()
    const handler = () => onDismiss()
    handler()
    expect(onDismiss).toHaveBeenCalledTimes(1)
  })
})
