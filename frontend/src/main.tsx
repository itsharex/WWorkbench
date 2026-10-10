import React from 'react'
import ReactDOM from 'react-dom'
import { createRoot } from 'react-dom/client'
import App from './App'
import { ErrorBoundary } from './components/ErrorBoundary'
import { installWWHost } from './plugin/host'

/** Pack IIFE external：同一份 React + WWHost 业务表面。 */
Object.assign(globalThis, { React, ReactDOM })
installWWHost()

createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ErrorBoundary>
      <App />
    </ErrorBoundary>
  </React.StrictMode>
)
