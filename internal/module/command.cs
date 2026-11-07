using System;
using System.IO;
using System.Text;
using System.Collections.Generic;
using System.Diagnostics;
using System.Threading.Tasks;
public class Receiver {
 string s; int i;
 void W(){while(i<s.Length && " \r\n\t".IndexOf(s[i])>=0)i++;}
 char C(){if(i==s.Length)throw new Exception();return s[i++];}
 string S(){if(C()!='"')throw new Exception();var b=new StringBuilder();for(;;){char c=C();if(c=='"')break;if(c<32)throw new Exception();if(c=='\\'){c=C();int n="\"\\/bfnrtu".IndexOf(c);if(n<0)throw new Exception();if(n==8){string hex=s.Substring(i,4);foreach(char h in hex)if(!Uri.IsHexDigit(h))throw new Exception();c=(char)Convert.ToInt32(hex,16);i+=4;}else c="\"\\/\b\f\n\r\t"[n];}b.Append(c);}string v=b.ToString();new UTF8Encoding(false,true).GetBytes(v);return v;}
 object V(int depth){if(depth>3)throw new Exception();W();char c=C();i--;if(c=='"')return S();if(c=='['){i++;var a=new List<string>();W();if(s[i]!=']')for(;;){W();a.Add(S());W();c=C();if(c==']')return a;if(c!=',')throw new Exception();}i++;return a;}if(c!='{')throw new Exception();i++;var d=new Dictionary<string,object>(StringComparer.Ordinal);W();if(s[i]!='}')for(;;){W();string k=S();W();if(C()!=':')throw new Exception();d.Add(k,V(depth+1));W();c=C();if(c=='}')return d;if(c!=',')throw new Exception();}i++;return d;}
 static string Q(string v){if(v.Length>0 && v.IndexOfAny(new char[]{' ','\t','\n','\v','"'})<0)return v;var b=new StringBuilder("\"");int n=0;foreach(char c in v){if(c=='\\'){n++;continue;}b.Append('\\',c=='"'?2*n+1:n);n=0;b.Append(c);}return b.Append('\\',2*n).Append('"').ToString();}
 static byte[] Read(Stream s,int max){using(var m=new MemoryStream()){byte[] b=new byte[8192];while(m.Length<max){int n=s.Read(b,0,(int)Math.Min(b.Length,max-m.Length));if(n==0)break;m.Write(b,0,n);}return m.ToArray();}}
 static string Drain(Stream s,out bool cut){using(var m=new MemoryStream()){byte[] b=new byte[8192];int n;cut=false;while((n=s.Read(b,0,b.Length))>0){int k=(int)Math.Min(n,1048576-m.Length);m.Write(b,0,k);if(k<n)cut=true;}return Convert.ToBase64String(m.ToArray());}}
 public static void Run(){string response="{\"error\":\"invalid_request\"}";bool started=false;try{
 byte[] input=Read(Console.OpenStandardInput(),2097153);if(input.Length>2097152)throw new Exception();var p=new Receiver();p.s=new UTF8Encoding(false,true).GetString(input);var d=(Dictionary<string,object>)p.V(0);p.W();if(p.i!=p.s.Length || d.Count!=2 || (string)d["module"]!="command")throw new Exception();var a=(Dictionary<string,object>)d["args"];if(a.Count!=2)throw new Exception();string exe=(string)a["executable"];var argv=(List<string>)a["argv"];
 if(!System.Text.RegularExpressions.Regex.IsMatch(exe,@"\A[A-Za-z]:\\[^\x00-\x1f\x7f-\x9f\""<>:|?*/]*\.exe\z",System.Text.RegularExpressions.RegexOptions.IgnoreCase) || Path.GetFileName(exe).Equals("cmd.exe",StringComparison.OrdinalIgnoreCase))throw new Exception();
 var line=new List<string>();foreach(string v in argv){if(v.IndexOf('\0')>=0)throw new Exception();line.Add(Q(v));}string args=String.Join(" ",line);if(("\""+exe+"\""+(argv.Count>0?" "+args:"")).Length>32766)throw new Exception();
 response="{\"error\":\"start_failed\"}";using(var child=new Process()){child.StartInfo=new ProcessStartInfo(exe,args){UseShellExecute=false,RedirectStandardOutput=true,RedirectStandardError=true,RedirectStandardInput=true,CreateNoWindow=true};if(!child.Start())throw new Exception();started=true;child.StandardInput.Close();bool oc=false,ec=false;var o=Task.Factory.StartNew(()=>Drain(child.StandardOutput.BaseStream,out oc));var e=Task.Factory.StartNew(()=>Drain(child.StandardError.BaseStream,out ec));child.WaitForExit();Task.WaitAll(o,e);response="{\"exit_code\":"+child.ExitCode+",\"stdout\":\""+o.Result+"\",\"stderr\":\""+e.Result+"\",\"stdout_truncated\":"+oc.ToString().ToLowerInvariant()+",\"stderr_truncated\":"+ec.ToString().ToLowerInvariant()+"}";}
 }catch{if(started)response="{\"error\":\"execution_failed\"}";}
 byte[] output=Encoding.UTF8.GetBytes(response);Console.OpenStandardOutput().Write(output,0,output.Length);}
}
