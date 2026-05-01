import { render, screen } from '@testing-library/react'
import { DiffTree } from '../components/DiffTree.tsx'
import type { DiffNode } from '../api/types.ts'

describe('DiffTree', () => {
  it('renders empty state when no root', () => {
    render(<DiffTree root={null} />)
    expect(screen.getByText('No diff available.')).toBeInTheDocument()
  })

  it('renders unchanged node', () => {
    const root: DiffNode = { kind: 'unchanged', key: 'a', oldValue: 1, newValue: 1 }
    render(<DiffTree root={root} />)
    expect(screen.getByText(/a:/)).toBeInTheDocument()
    expect(screen.getByText('1')).toBeInTheDocument()
  })

  it('renders changed node with old and new values', () => {
    const root: DiffNode = { kind: 'changed', key: 'b', oldValue: 2, newValue: 3 }
    render(<DiffTree root={root} />)
    expect(screen.getByText(/b:/)).toBeInTheDocument()
    expect(screen.getByText('2')).toBeInTheDocument()
    expect(screen.getByText('3')).toBeInTheDocument()
  })

  it('renders added node', () => {
    const root: DiffNode = { kind: 'added', key: 'c', newValue: 4 }
    render(<DiffTree root={root} />)
    expect(screen.getByText(/c:/)).toBeInTheDocument()
    expect(screen.getByText('4')).toBeInTheDocument()
  })

  it('renders removed node', () => {
    const root: DiffNode = { kind: 'removed', key: 'd', oldValue: 5 }
    render(<DiffTree root={root} />)
    expect(screen.getByText(/d:/)).toBeInTheDocument()
    expect(screen.getByText('5')).toBeInTheDocument()
  })

  it('renders nested children', () => {
    const root: DiffNode = {
      kind: 'changed',
      children: [
        { kind: 'unchanged', key: 'x', oldValue: 1, newValue: 1 },
        { kind: 'added', key: 'y', newValue: 2 },
      ],
    }
    render(<DiffTree root={root} />)
    expect(screen.getByText(/x:/)).toBeInTheDocument()
    expect(screen.getByText(/y:/)).toBeInTheDocument()
  })
})
