export const API = import.meta.env.VITE_API_BASE || "/api";
export const WS = import.meta.env.VITE_WS_BASE || `${location.protocol === "https:" ? "wss" : "ws"}://${location.host}/ws`;

export async function api(path:string, init:RequestInit={}) {
  const headers = new Headers(init.headers);
  headers.set("Content-Type","application/json");
  const token = localStorage.getItem("pp_token");
  if (token) headers.set("Authorization",`Bearer ${token}`);
  const res = await fetch(API+path,{...init,headers});
  const data = await res.json().catch(()=>({}));
  if (!res.ok) throw new Error(data.error || "Request failed");
  return data;
}
