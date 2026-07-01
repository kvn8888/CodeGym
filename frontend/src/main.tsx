import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App'
import { CodeGymAuthProvider } from './shared/auth/AuthProvider'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <CodeGymAuthProvider>
      <App />
    </CodeGymAuthProvider>
  </StrictMode>,
)
