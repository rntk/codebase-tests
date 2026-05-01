import { render, screen, fireEvent } from '@testing-library/react'
import { TreeView } from '../components/TreeView.tsx'

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

  it('calls onSelect when clicking a node', () => {
    const onSelect = vi.fn()
    render(<TreeView nodes={nodes} onSelect={onSelect} />)
    fireEvent.click(screen.getByTestId('tree-item-child1'))
    expect(onSelect).toHaveBeenCalledWith('child1')
  })

  it('highlights selected node', () => {
    render(<TreeView nodes={nodes} selectedId="child1" />)
    const item = screen.getByTestId('tree-item-child1')
    expect(item.style.background).toBe('rgb(227, 242, 253)')
  })
})
