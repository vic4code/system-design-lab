import type { Metadata } from "next";
import "./globals.css";
import { PlayerProvider } from "@/context/PlayerContext";
import Navbar from "@/components/Navbar";
import Player from "@/components/Player";

export const metadata: Metadata = {
  title: "Beatstream",
  description: "Spotify-like audio streaming — system design lab",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body className="min-h-screen bg-bg text-white">
        <PlayerProvider>
          <Navbar />
          <main className="pb-24">{children}</main>
          <Player />
        </PlayerProvider>
      </body>
    </html>
  );
}
