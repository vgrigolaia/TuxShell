import { mount } from 'svelte'
import '@fontsource-variable/geist'
import '@fontsource-variable/space-grotesk'
import '@fontsource-variable/jetbrains-mono'
import '@xterm/xterm/css/xterm.css'
import './app.css'
import App from './App.svelte'

mount(App, { target: document.getElementById('app')! })
