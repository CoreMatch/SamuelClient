import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import GetStartedChecklist from './GetStartedChecklist';

describe('GetStartedChecklist', () => {
  it('lists the remaining steps for a fresh install', () => {
    render(<GetStartedChecklist hasAccount={false} installedCount={0} hasLaunched={false} />);
    expect(screen.getByText('Get started')).toBeTruthy();
    expect(screen.getByText('Sign in')).toBeTruthy();
    expect(screen.getByText('Install a version')).toBeTruthy();
    expect(screen.getByText('Launch the game')).toBeTruthy();
  });

  it('drops completed steps', () => {
    render(<GetStartedChecklist hasAccount installedCount={1} hasLaunched={false} />);
    // Completed items stay visible but struck through; hints disappear.
    expect(screen.queryByText('Add an account to play online or offline.')).toBeNull();
    expect(screen.queryByText('Download Minecraft from the Versions page.')).toBeNull();
    expect(screen.getByText('Pick your version and press Play.')).toBeTruthy();
  });

  it('disappears once everything is done', () => {
    const { container } = render(<GetStartedChecklist hasAccount installedCount={2} hasLaunched />);
    expect(container.firstChild).toBeNull();
  });
});
