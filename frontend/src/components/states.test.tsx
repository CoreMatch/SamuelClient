import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { LoadingSkeleton, ErrorState, EmptyState } from './states';

describe('unified states', () => {
  it('renders skeleton rows while loading', () => {
    const { container } = render(<LoadingSkeleton rows={4} />);
    expect(container.querySelectorAll('.skeleton-row').length).toBe(4);
  });

  it('shows a readable error with a retry action', () => {
    const onRetry = vi.fn();
    render(<ErrorState data="the version list" onRetry={onRetry} />);

    expect(
      screen.getByText("Couldn't load the version list. Check your connection and try again."),
    ).toBeTruthy();
    fireEvent.click(screen.getByText('Retry'));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it('shows an empty state with a next-step action', () => {
    const onAction = vi.fn();
    render(
      <EmptyState title="No versions installed" hint="Download one." actionLabel="Browse versions" onAction={onAction} />,
    );

    expect(screen.getByText('No versions installed')).toBeTruthy();
    expect(screen.getByText('Download one.')).toBeTruthy();
    fireEvent.click(screen.getByText('Browse versions'));
    expect(onAction).toHaveBeenCalledTimes(1);
  });
});
