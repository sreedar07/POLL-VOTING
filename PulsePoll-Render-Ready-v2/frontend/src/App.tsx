import { useEffect, useState } from "react";
import { Link, Route, Routes, useNavigate, useParams } from "react-router-dom";
import { api, WS } from "./api";

type Option = { id:string; text:string; emoji:string; votes:number; percent:number };
type Snapshot = { slug:string; totalVotes:number; options:Option[]; status:string };
type Poll = {
  slug:string; question:string; options:{id:string;text:string;emoji:string}[];
  multiple:boolean; restrictDuplicates:boolean; pulse:boolean; status:string;
  durationSeconds:number; maxVotes:number; createdAt:string; closedAt?:string;
};

function Shell({children}:{children:React.ReactNode}) {
  const [theme,setTheme]=useState(localStorage.getItem("theme")||"system");
  useEffect(()=>{document.documentElement.dataset.theme=theme;localStorage.setItem("theme",theme)},[theme]);
  return <div className="app">
    <header><Link className="brand" to="/"><span className="logo">✦</span> PulsePoll</Link>
      <nav><Link to="/">Create</Link><Link to="/login">Creator Login</Link>
      <button className="theme" onClick={()=>setTheme(theme==="dark"?"light":theme==="light"?"system":"dark")}>◐</button></nav>
    </header><main>{children}</main><footer>PulsePoll · guest voting · real-time WebSockets · Redis</footer>
  </div>;
}

function Home(){
 const nav=useNavigate(); const [question,setQuestion]=useState(""); const [options,setOptions]=useState(["",""]);
 const [duration,setDuration]=useState("0"); const [maxVotes,setMaxVotes]=useState("0");
 const [multiple,setMultiple]=useState(false); const [restrict,setRestrict]=useState(true);
 const [err,setErr]=useState(""); const [busy,setBusy]=useState(false);
 const submit=async(e:React.FormEvent)=>{e.preventDefault();setErr("");setBusy(true);
  try{const d=await api("/polls",{method:"POST",body:JSON.stringify({question,options,durationSeconds:Number(duration),maxVotes:Number(maxVotes),multiple,restrictDuplicates:restrict,pulse:true})});
   nav(`/poll/${d.poll.slug}/share`)}catch(e:any){setErr(e.message)}finally{setBusy(false)}};
 return <section className="hero-grid"><div className="hero"><span className="eyebrow">REAL-TIME • ZERO FRICTION</span>
  <h1>Ask a question.<br/><em>Watch the room respond.</em></h1>
  <p>Create a live poll, share one URL or QR code, and watch results update instantly. Voters need no account.</p>
  <div className="hero-pills"><span>⚡ Redis realtime</span><span>🔒 Guest voting</span><span>📱 QR sharing</span></div></div>
  <form className="card creator" onSubmit={submit}><div className="card-head"><div><span className="eyebrow">NEW POLL</span><h2>Create your poll</h2></div><span className="secure">Creator login required</span></div>
   <label>Question<input required value={question} onChange={e=>setQuestion(e.target.value)} placeholder="What should we build next?" maxLength={240}/></label>
   <div className="label-row"><label>Answer options <small>{options.length}/8</small></label><button type="button" className="ghost" onClick={()=>options.length<8&&setOptions([...options,""])}>+ Add</button></div>
   {options.map((v,i)=><div className="option-input" key={i}><span>{i+1}</span><input required value={v} onChange={e=>setOptions(options.map((x,j)=>j===i?e.target.value:x))} placeholder={`Option ${i+1}`}/>{options.length>2&&<button type="button" onClick={()=>setOptions(options.filter((_,j)=>j!==i))}>×</button>}</div>)}
   <div className="settings">
    <label>Time limit<select value={duration} onChange={e=>setDuration(e.target.value)}><option value="0">No limit</option><option value="30">30 seconds</option><option value="60">1 minute</option><option value="300">5 minutes</option><option value="900">15 minutes</option><option value="3600">1 hour</option><option value="86400">1 day</option><option value="604800">7 days</option></select></label>
    <label>Max votes<select value={maxVotes} onChange={e=>setMaxVotes(e.target.value)}><option value="0">Unlimited</option><option value="10">10</option><option value="25">25</option><option value="50">50</option><option value="100">100</option><option value="500">500</option><option value="1000">1,000</option></select></label>
    <label className="check"><input type="checkbox" checked={multiple} onChange={e=>setMultiple(e.target.checked)}/> Multiple choices</label>
    <label className="check"><input type="checkbox" checked={restrict} onChange={e=>setRestrict(e.target.checked)}/> Duplicate protection</label>
   </div>
   {err&&<div className="error">{err} — <Link to="/login">log in as creator</Link></div>}
   <button className="primary" disabled={busy}>{busy?"Creating…":"Create live poll →"}</button>
  </form></section>;
}

function Auth({mode}:{mode:"login"|"signup"}){
 const nav=useNavigate();const [email,setEmail]=useState("");const [password,setPassword]=useState("");const [err,setErr]=useState("");
 const submit=async(e:React.FormEvent)=>{e.preventDefault();try{const d=await api(`/auth/${mode==="login"?"login":"signup"}`,{method:"POST",body:JSON.stringify({email,password})});localStorage.setItem("pp_token",d.token);nav("/dashboard")}catch(e:any){setErr(e.message)}};
 return <section className="center"><form className="card auth"><span className="eyebrow">CREATOR ACCESS</span><h2>{mode==="login"?"Welcome back":"Create creator account"}</h2><p>Voters never need an account. This account protects poll creation and management.</p>
  <label>Email<input type="email" required value={email} onChange={e=>setEmail(e.target.value)}/></label><label>Password<input type="password" minLength={8} required value={password} onChange={e=>setPassword(e.target.value)}/></label>
  {err&&<div className="error">{err}</div>}<button className="primary">{mode==="login"?"Log in":"Sign up"} →</button>
  <Link to={mode==="login"?"/signup":"/login"} className="switch">{mode==="login"?"Need an account? Sign up":"Already registered? Log in"}</Link>
 </form></section>;
}

function usePoll(slug:string){
 const [data,setData]=useState<{poll:Poll;snapshot:Snapshot}|null>(null);const [online,setOnline]=useState(false);
 useEffect(()=>{
  let stopped=false;let timer:number|undefined;
  const load=()=>api(`/polls/${slug}`).then(d=>{if(!stopped)setData(d)}).catch(()=>{});
  const connect=()=>{
   if(stopped)return;
   const ws=new WebSocket(`${WS}?poll=${encodeURIComponent(slug)}`);
   ws.onopen=()=>setOnline(true);
   ws.onmessage=e=>{const m=JSON.parse(e.data);if(m.poll)setData(x=>x?{...x,snapshot:m.poll}:x)};
   ws.onclose=()=>{setOnline(false);if(!stopped)timer=window.setTimeout(connect,2000)};
   ws.onerror=()=>ws.close();
   return ws;
  };
  load();const ws=connect();return()=>{stopped=true;if(timer)clearTimeout(timer);ws.close()};
 },[slug]);
 return {data,online};
}

function VotingPage(){
 const {slug}=useParams();const nav=useNavigate();const {data,online}=usePoll(slug!);
 const [selected,setSelected]=useState<string[]>([]);const [err,setErr]=useState("");const [busy,setBusy]=useState(false);
 if(!data)return <section className="center"><div className="loader">Loading poll…</div></section>;
 const {poll,snapshot}=data;
 const vote=async()=>{setErr("");setBusy(true);try{await api(`/polls/${slug}/vote`,{method:"POST",body:JSON.stringify({optionIds:selected})});nav(`/poll/${slug}/results`)}catch(e:any){setErr(e.message)}finally{setBusy(false)}};
 const toggle=(id:string)=>setSelected(s=>poll.multiple?(s.includes(id)?s.filter(x=>x!==id):[...s,id]):[id]);
 return <section className="page"><div className="poll-top"><span className={online?"live":"offline"}>● {online?"LIVE":"RECONNECTING"}</span><span>{snapshot.totalVotes.toLocaleString()} votes</span></div>
  <div className="center"><div className="card vote-card"><span className="eyebrow">{poll.multiple?"MULTIPLE CHOICE":"SINGLE CHOICE"}</span><h1>{poll.question}</h1>
   {snapshot.status!=="open"?<div className="closed">Voting is closed. <Link to={`/poll/${slug}/results`}>View final results →</Link></div>:
   <>{poll.options.map(o=>{const s=selected.includes(o.id);return <button className={"vote-option "+(s?"selected":"")} key={o.id} onClick={()=>toggle(o.id)}><span className="radio">{s?"✓":""}</span><span>{o.text}</span></button>})}
   <button className="primary vote-btn" disabled={!selected.length||busy} onClick={vote}>{busy?"Submitting…":"Cast vote →"}</button>{err&&<div className="error">{err}</div>}</>}
  </div></div>
 </section>;
}

function ResultsPage({presenter=false}:{presenter?:boolean}){
 const {slug}=useParams();const {data,online}=usePoll(slug!);
 if(!data)return <section className="center"><div className="loader">Loading live results…</div></section>;
 const {poll,snapshot}=data;const closed=snapshot.status!=="open";const sorted=[...snapshot.options].sort((a,b)=>b.votes-a.votes);
 const top=sorted[0]?.votes||0;const winners=top>0?sorted.filter(o=>o.votes===top):[];
 return <section className={presenter?"presenter page":"page"}><div className="poll-top"><span className={closed?"closed-pill":online?"live":"offline"}>● {closed?"FINAL":online?"LIVE":"RECONNECTING"}</span><span>{snapshot.totalVotes.toLocaleString()} votes{poll.maxVotes>0?` / ${poll.maxVotes} max`:""}</span></div>
  <div className="results-layout"><div className="card results-main">
   <span className="eyebrow">{closed?"FINAL RESULTS":"LIVE RESULTS"}</span><h1>{poll.question}</h1>
   {closed&&winners.length>0&&<div className="winner"><span className="trophy">🏆</span><div><small>{winners.length>1?"TIE":"WINNER"}</small><strong>{winners.map(w=>w.text).join(" · ")}</strong><span>{top} vote{top!==1?"s":""} · {winners[0].percent.toFixed(2)}%</span></div></div>}
   {closed&&snapshot.totalVotes===0&&<div className="closed">No votes were recorded.</div>}
   {sorted.map(o=><div className={"result "+(closed&&o.votes===top&&top>0?"winner-row":"")} key={o.id}><div className="result-label"><span>{o.emoji||"◈"} {o.text}</span><b>{o.percent.toFixed(2)}% <small>({o.votes})</small></b></div><div className="bar"><span style={{width:`${Math.min(100,Math.max(0,o.percent))}%`}}/></div></div>)}
   {!closed&&<div className="activity"><strong>Live connection</strong><div>New votes appear automatically — no refresh required.</div></div>}
   {closed&&<div className="share-actions"><Link className="secondary" to={`/poll/${slug}`}>Voting page</Link><Link className="secondary" to={`/poll/${slug}/present`}>Presenter mode</Link></div>}
  </div></div>
 </section>;
}

function Share(){
 const {slug}=useParams();const url=`${location.origin}/poll/${slug}`;const [qr,setQr]=useState("");const [copied,setCopied]=useState(false);
 useEffect(()=>{import("qrcode").then(q=>q.toDataURL(url,{width:280,margin:1})).then(setQr)},[url]);
 return <section className="center"><div className="card share"><span className="eyebrow">POLL READY</span><h1>Share the room.</h1><p>Send the unique URL or scan the QR code. Friends can vote without creating an account.</p>
  <div className="url-box"><input readOnly value={url}/><button onClick={()=>navigator.clipboard.writeText(url).then(()=>setCopied(true))}>{copied?"Copied":"Copy"}</button></div>
  <div className="share-grid"><a href={`https://twitter.com/intent/tweet?text=${encodeURIComponent("Vote live: "+url)}`} target="_blank">𝕏 Share</a><a href={`https://www.linkedin.com/sharing/share-offsite/?url=${encodeURIComponent(url)}`} target="_blank">in Share</a><a href={`mailto:?subject=Live poll&body=${encodeURIComponent(url)}`}>✉ Email</a></div>
  {qr&&<img className="qr" src={qr} alt="QR code for poll invitation"/>}
  <div className="share-actions"><Link className="primary" to={`/poll/${slug}`}>Open voting page</Link><Link className="secondary" to={`/poll/${slug}/results`}>Live results</Link><Link className="secondary" to="/dashboard">Dashboard</Link></div>
 </div></section>;
}

function Dashboard(){
 const [polls,setPolls]=useState<Poll[]>([]);const [err,setErr]=useState("");
 const load=()=>api("/admin/polls").then(d=>setPolls(d.polls)).catch(e=>setErr(e.message));useEffect(load,[]);
 const close=async(slug:string)=>{await api(`/admin/polls/${slug}/close`,{method:"POST"});load()};
 const del=async(slug:string)=>{if(confirm("Delete this poll?")){await api(`/admin/polls/${slug}`,{method:"DELETE"});load()}};
 return <section className="dashboard"><div className="dash-head"><div><span className="eyebrow">CREATOR CONSOLE</span><h1>Your polls</h1></div><Link className="primary" to="/">+ New poll</Link></div>
  {err&&<div className="error">{err}</div>}<div className="poll-list">{polls.map(p=><div className="card poll-row" key={p.slug}><div><span className={p.status==="open"?"live":"closed-pill"}>{p.status}</span><h3>{p.question}</h3><small>/poll/{p.slug}</small></div>
   <div className="row-actions"><Link to={`/poll/${p.slug}`}>Vote</Link><Link to={`/poll/${p.slug}/results`}>Results</Link>{p.status==="open"&&<button onClick={()=>close(p.slug)}>Close</button>}<button onClick={()=>del(p.slug)}>Delete</button><a href={`${location.origin}/api/admin/polls/${p.slug}/export?format=csv`}>CSV</a></div>
  </div>)}</div></section>;
}

export default function App(){return <Shell><Routes>
 <Route path="/" element={<Home/>}/><Route path="/login" element={<Auth mode="login"/>}/><Route path="/signup" element={<Auth mode="signup"/>}/><Route path="/dashboard" element={<Dashboard/>}/>
 <Route path="/poll/:slug/share" element={<Share/>}/><Route path="/poll/:slug/present" element={<ResultsPage presenter/>}/><Route path="/poll/:slug/results" element={<ResultsPage/>}/><Route path="/poll/:slug" element={<VotingPage/>}/>
 </Routes></Shell>}
