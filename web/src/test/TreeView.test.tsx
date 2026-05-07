import * as fs from 'fs'
import * as path from 'path'
import { fileURLToPath } from 'url'
import { render, screen, fireEvent } from '@testing-library/react'
import { TreeView } from '../components/TreeView.tsx'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)

describe('TreeView', () => {
  const nodes = [
    {
      id: 'root',
      label: 'Root',
      children: [
        { id: 'child1', label: 'Child 1' },
        { id: 'child2', label: 'Child 2' },
      ],
    },
  ]

  it('renders root nodes', () => {
    render(<TreeView nodes={nodes} />)
    expect(screen.getByText('Root')).toBeInTheDocument()
  })

  it('expands and collapses children', () => {
    render(<TreeView nodes={nodes} />)
    expect(screen.getByText('Child 1')).toBeInTheDocument()
    fireEvent.click(screen.getByTestId('tree-item-root'))
    expect(screen.queryByText('Child 1')).not.toBeInTheDocument()
  })

  describe('calls onSelect when clicking a node', () => {
    const goldenDir = path.resolve(
      __dirname,
      '../../../tests/golden/web/src/test/TreeView',
    )
    const cases = fs
      .readdirSync(goldenDir)
      .filter((f) => f.startsWith('calls_onSelect_when_clicking_a_node') && f.endsWith('.in.json'))
      .map((f) => f.replace('.in.json', ''))

    it.each(cases)('%s', (caseName) => {
      const inFile = path.join(goldenDir, `${caseName}.in.json`)
      const outFile = path.join(goldenDir, `${caseName}.out.json`)
      const input = JSON.parse(fs.readFileSync(inFile, 'utf-8'))
      const expected = JSON.parse(fs.readFileSync(outFile, 'utf-8'))

      const onSelect = vi.fn()
      render(<TreeView nodes={input.nodes} onSelect={onSelect} />)
      fireEvent.click(screen.getByTestId(input.clickTarget))

      expect(onSelect).toHaveBeenCalledWith(expected.onSelectCalledWith)
    })
  })

  it('highlights selected node', () => {
    render(<TreeView nodes={nodes} selectedId="child1" />)
    const item = screen.getByTestId('tree-item-child1')
    expect(item.style.background).toBe('rgb(187, 222, 251)')
  })

  it('renders node markers', () => {
    render(
      <TreeView
        nodes={[
          {
            id: 'test-with-golden',
            label: 'TestWithGolden',
            marker: { kind: 'success', label: 'Has golden test data' },
          },
        ]}
      />
    )

    const marker = screen.getByTestId('tree-item-marker-test-with-golden')
    expect(marker).toHaveTextContent('✓')
    expect(marker).toHaveAccessibleName('Has golden test data')
  })

  it('renders changed node markers', () => {
    render(
      <TreeView
        nodes={[
          {
            id: 'test-with-changes',
            label: 'TestWithChanges',
            marker: { kind: 'changed', label: 'Has changes' },
          },
        ]}
      />
    )

    const marker = screen.getByTestId('tree-item-marker-test-with-changes')
    expect(marker).toHaveTextContent('Δ')
    expect(marker).toHaveAccessibleName('Has changes')
  })
})
