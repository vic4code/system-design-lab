import type { Metadata } from "next";
import "./globals.css";
import { PlayerProvider } from "@/context/PlayerContext";
import Sidebar from "@/components/Sidebar";
import Player from "@/components/Player";

export const metadata: Metadata = {
  title: "Beatstream",
  description: "Spotify-like audio streaming — system design lab",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className="h-full">
      <body className="h-full flex flex-col bg-bg text-white overflow-hidden">
        <PlayerProvider>
          <div className="flex flex-1 overflow-hidden">
            <Sidebar />
            <main className="flex-1 overflow-y-auto bg-bg">
              {children}
            </main>
          </div>
          <Player />
        </PlayerProvider>
      </body>
    </html>
  );
}
