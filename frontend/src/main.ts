import './style.css'
import './app.css'
import App from './App.svelte'
import { mount } from 'svelte'

const target = document.getElementById('app')!
export default mount(App, { target })
