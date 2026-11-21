import { describe, expect, it } from 'vitest';
import { recommendJavaMajor } from './javaRuntimeRecommendation';

describe('recommendJavaMajor', () => {
  it.each([
    { gameVersion: '1.12.2', javaMajor: 8 },
    { gameVersion: '1.16.5', javaMajor: 8 },
    { gameVersion: '1.17.1', javaMajor: 17 },
    { gameVersion: '1.18.2', javaMajor: 17 },
    { gameVersion: '1.20.4', javaMajor: 17 },
    { gameVersion: '1.20.5', javaMajor: 21 },
    { gameVersion: '1.21.4', javaMajor: 21 },
    { gameVersion: '26.1', javaMajor: 25 },
    { gameVersion: '26.2', javaMajor: 25 },
  ])('recommends Java $javaMajor for Minecraft $gameVersion', ({ gameVersion, javaMajor }) => {
    expect(recommendJavaMajor(gameVersion)?.major).toBe(javaMajor);
  });

  it('marks Java 17 as the LTS recommendation for 1.17', () => {
    expect(recommendJavaMajor('1.17.1')).toEqual({ major: 17, note: 'java17LtsFor117' });
  });

  it.each(['', 'latest', '1.22.0', '26.0', 'garbage'])(
    'has no recommendation for unknown game version %s',
    gameVersion => {
      expect(recommendJavaMajor(gameVersion)).toBeNull();
    },
  );
});
