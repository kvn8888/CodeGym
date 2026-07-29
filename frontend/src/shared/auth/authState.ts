import { createContext, useContext } from 'react';

export interface CodeGymAuthState {
  configured: boolean;
  isAuthenticated: boolean;
}

export const CodeGymAuthContext = createContext<CodeGymAuthState>({
  configured: false,
  isAuthenticated: true,
});

export function useCodeGymAuthState() {
  return useContext(CodeGymAuthContext);
}
