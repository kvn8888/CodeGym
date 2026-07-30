import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { ThemeProvider } from './components/theme-provider';
import { TooltipProvider } from './components/ui/tooltip';
import { Layout } from './shared/components/Layout';
import { ProblemListPage } from './features/problems/ProblemListPage';
import { ProblemDetailPage } from './features/problems/ProblemDetailPage';
import { GeneratePage } from './features/generate/GeneratePage';
import { DashboardPage } from './features/dashboard/DashboardPage';
import { MarathonPage } from './features/marathon/MarathonPage';
import { MemoryPage } from './features/memory/MemoryPage';
import { SettingsPage } from './features/settings/SettingsPage';
import { InterviewPage } from './features/interview/InterviewPage';

function App() {
  return (
    <ThemeProvider defaultTheme="light">
      <TooltipProvider delayDuration={200}>
        <BrowserRouter>
          <Routes>
            <Route element={<Layout />}>
              <Route path="/" element={<ProblemListPage />} />
              <Route path="/problems/:id" element={<ProblemDetailPage />} />
              <Route path="/generate" element={<GeneratePage />} />
              <Route path="/marathon" element={<MarathonPage />} />
              <Route path="/interviews/:sessionId" element={<InterviewPage />} />
              <Route path="/memory" element={<MemoryPage />} />
              <Route path="/dashboard" element={<DashboardPage />} />
              <Route path="/settings" element={<SettingsPage />} />
            </Route>
          </Routes>
        </BrowserRouter>
      </TooltipProvider>
    </ThemeProvider>
  );
}

export default App;
