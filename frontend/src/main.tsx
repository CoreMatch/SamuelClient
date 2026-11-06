import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
// Fonts are bundled locally (no CDN), per the @ha/ui consumer contract:
// Inter for UI text, JetBrains Mono for numbers, paths and log output.
import '@fontsource/inter/400.css';
import '@fontsource/inter/500.css';
import '@fontsource/inter/600.css';
import '@fontsource/inter/700.css';
import '@fontsource/jetbrains-mono/400.css';
import '@fontsource/jetbrains-mono/600.css';
import '@fontsource/jetbrains-mono/700.css';
import './index.css';
import App from './App';

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
