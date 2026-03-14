import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { Layout } from './shared/components/Layout';
import { ProblemListPage } from './features/problems/ProblemListPage';
import { ProblemDetailPage } from './features/problems/ProblemDetailPage';
import { GeneratePage } from './features/generate/GeneratePage';
import { DashboardPage } from './features/dashboard/DashboardPage';
import { ChatPage } from './features/chat/ChatPage';
import { MarathonPage } from './features/marathon/MarathonPage';

function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route element={<Layout />}>
          <Route path="/" element={<ProblemListPage />} />
          <Route path="/problems/:id" element={<ProblemDetailPage />} />
          <Route path="/generate" element={<GeneratePage />} />
          <Route path="/chat" element={<ChatPage />} />
          <Route path="/marathon" element={<MarathonPage />} />
          <Route path="/dashboard" element={<DashboardPage />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}

export default App;
