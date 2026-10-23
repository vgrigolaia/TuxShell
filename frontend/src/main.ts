import { mount } from 'svelte'
import '@fontsource-variable/geist'
import '@fontsource-variable/space-grotesk'
import '@fontsource-variable/jetbrains-mono'
import '@xterm/xterm/css/xterm.css'
import './app.css'
import { showStartupError } from './lib/startupError'

// Loaded with import(), so an error while the app's modules load shows on screen
// instead of an empty window (1.4.1 and 1.4.2 on macOS).
import('./App.svelte')
  .then(({ default: App }) => mount(App, { target: document.getElementById('app')! }))
  .catch(showStartupError)
