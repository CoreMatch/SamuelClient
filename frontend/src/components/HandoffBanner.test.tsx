import { describe, it, expect, vi } from 'vitest';
import type { ComponentProps } from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import HandoffBanner from './HandoffBanner';
import { HandoffPayload } from '../types';

const handoff: HandoffPayload = {
  version: 1,
  handoff_id: '9b2f0c1e-3a7d-4e5f-8b21-6c9d0e1f2a3b',
  expires_at: '2099-01-01T00:00:00Z',
  match: {
    match_id: 'm-1',
    mode: 'bedwars',
    map_name: 'Aquarium',
    join_target: { host: 'bw.match.local', port: 25565, game_version: '1.20.4' },
  },
};

function defaultProps(overrides: Partial<ComponentProps<typeof HandoffBanner>> = {}) {
  return {
    handoff: null as HandoffPayload | null,
    notice: '',
    hasAccount: true,
    joining: false,
    error: '',
    onJoin: vi.fn(),
    onLogin: vi.fn(),
    onDismiss: vi.fn(),
    ...overrides,
  };
}

describe('HandoffBanner', () => {
  it('renders nothing without a handoff or notice', () => {
    const { container } = render(<HandoffBanner {...defaultProps()} />);
    expect(container.firstChild).toBeNull();
  });

  it('shows match details and Join now when an account is logged in', () => {
    const props = defaultProps({ handoff });
    render(<HandoffBanner {...props} />);

    expect(screen.getByText('Match found')).toBeTruthy();
    expect(screen.getByText(/Mode: bedwars/)).toBeTruthy();
    expect(screen.getByText(/Server: bw\.match\.local:25565/)).toBeTruthy();

    fireEvent.click(screen.getByText('Join now'));
    expect(props.onJoin).toHaveBeenCalledTimes(1);
  });

  it('asks for login when no account is available', () => {
    const props = defaultProps({ handoff, hasAccount: false });
    render(<HandoffBanner {...props} />);

    expect(screen.getByText('Log in to join this match')).toBeTruthy();
    fireEvent.click(screen.getByText('Log in'));
    expect(props.onLogin).toHaveBeenCalledTimes(1);
    expect(props.onJoin).not.toHaveBeenCalled();
  });

  it('shows a launch failure readably and keeps the handoff for retry', () => {
    const props = defaultProps({ handoff, error: 'Could not join the match: server down' });
    render(<HandoffBanner {...props} />);

    expect(screen.getByText(/Could not join the match: server down/)).toBeTruthy();
    // Join now stays available as the retry action.
    fireEvent.click(screen.getByText('Join now'));
    expect(props.onJoin).toHaveBeenCalledTimes(1);
  });

  it('shows a readable notice with OK when the handoff degraded', () => {
    const props = defaultProps({ notice: 'The matchmaking client no longer holds this match handoff.' });
    render(<HandoffBanner {...props} />);

    expect(screen.getByText(/no longer holds this match handoff/)).toBeTruthy();
    fireEvent.click(screen.getByText('OK'));
    expect(props.onDismiss).toHaveBeenCalledTimes(1);
  });

  it('dismisses the pending handoff', () => {
    const props = defaultProps({ handoff });
    render(<HandoffBanner {...props} />);

    fireEvent.click(screen.getByText('Dismiss'));
    expect(props.onDismiss).toHaveBeenCalledTimes(1);
  });

  it('disables actions while joining', () => {
    render(<HandoffBanner {...defaultProps({ handoff, joining: true })} />);
    expect(screen.getByText('Joining match…')).toBeTruthy();
    expect((screen.getByText('Joining match…').closest('button') as HTMLButtonElement).disabled).toBe(true);
  });

  it('shows the player list and a live expiry countdown', () => {
    const future = new Date(Date.now() + 5 * 60 * 1000).toISOString();
    render(
      <HandoffBanner
        {...defaultProps({
          handoff: {
            ...handoff,
            expires_at: future,
            match: {
              ...handoff.match,
              players: [
                { username: 'alice', uuid: 'u1' },
                { username: 'bob', uuid: 'u2' },
              ],
            },
          },
        })}
      />,
    );
    expect(screen.getByText('Players:')).toBeTruthy();
    expect(screen.getByText('alice')).toBeTruthy();
    expect(screen.getByText('bob')).toBeTruthy();
    expect(screen.getByText(/Expires in \d+:\d\d/)).toBeTruthy();
  });

  it('hides join and explains when the handoff has expired', () => {
    render(
      <HandoffBanner
        {...defaultProps({ handoff: { ...handoff, expires_at: '2020-01-01T00:00:00Z' } })}
      />,
    );
    expect(screen.getByText(/has expired/)).toBeTruthy();
    expect(screen.queryByText('Join now')).toBeNull();
  });
});
