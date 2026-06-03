"use client";

import { createContext, useContext, useEffect, useState, useCallback } from "react";
import { getFavoriteIDs, addFavorite, removeFavorite } from "@/lib/api";
import { useAuth } from "./AuthContext";

type FavoritesContextType = {
  ids: Set<string>;
  isFavorite: (trackId: string) => boolean;
  toggle: (trackId: string) => void;
};

const FavoritesContext = createContext<FavoritesContextType>({
  ids: new Set(),
  isFavorite: () => false,
  toggle: () => {},
});

export function FavoritesProvider({ children }: { children: React.ReactNode }) {
  const [ids, setIds] = useState<Set<string>>(new Set());
  const { state } = useAuth();

  useEffect(() => {
    if (state.status !== "authenticated") {
      setIds(new Set());
      return;
    }
    getFavoriteIDs()
      .then((r) => setIds(new Set(r.ids)))
      .catch(() => {});
  }, [state.status]);

  const isFavorite = useCallback((trackId: string) => ids.has(trackId), [ids]);

  const toggle = useCallback(
    (trackId: string) => {
      if (state.status !== "authenticated") return;
      if (ids.has(trackId)) {
        setIds((prev) => {
          const next = new Set(prev);
          next.delete(trackId);
          return next;
        });
        removeFavorite(trackId).catch(() => {
          setIds((prev) => new Set(prev).add(trackId));
        });
      } else {
        setIds((prev) => new Set(prev).add(trackId));
        addFavorite(trackId).catch(() => {
          setIds((prev) => {
            const next = new Set(prev);
            next.delete(trackId);
            return next;
          });
        });
      }
    },
    [ids, state.status]
  );

  return (
    <FavoritesContext.Provider value={{ ids, isFavorite, toggle }}>
      {children}
    </FavoritesContext.Provider>
  );
}

export const useFavorites = () => useContext(FavoritesContext);
