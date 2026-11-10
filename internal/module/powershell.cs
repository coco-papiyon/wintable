public partial class Receiver {
 [DllImport("kernel32.dll",EntryPoint="CreateDirectoryW",CharSet=CharSet.Unicode,SetLastError=true)]static extern bool MakeDirectory(string path,IntPtr security);
 static string Q(string v){if(v.Length>0 && v.IndexOfAny(new char[]{' ','\t','\n','\v','"'})<0)return v;var b=new StringBuilder("\"");int n=0;foreach(char c in v){if(c=='\\'){n++;continue;}b.Append('\\',c=='"'?2*n+1:n);n=0;b.Append(c);}return b.Append('\\',2*n).Append('"').ToString();}
 static string Drain(Stream s,out bool cut){using(var m=new MemoryStream()){byte[] b=new byte[8192];int n;cut=false;while((n=s.Read(b,0,b.Length))>0){int k=(int)Math.Min(n,1048576-m.Length);m.Write(b,0,k);if(k<n)cut=true;}return Convert.ToBase64String(m.ToArray());}}
 public static void Run(){string response="{\"error\":\"invalid_request\"}";bool started=false;string temp=null;try{
 byte[] input=Read(Console.OpenStandardInput(),2097153);if(input.Length>2097152)throw new Exception();var p=new Receiver();p.s=new UTF8Encoding(false,true).GetString(input);var d=(Dictionary<string,object>)p.V(0);p.W();if(p.i!=p.s.Length || d.Count!=2 || (string)d["module"]!="powershell")throw new Exception();var a=(Dictionary<string,object>)d["args"];if(a.Count!=1)throw new Exception();string script=(string)a["script"];
 if(String.IsNullOrWhiteSpace(script) || script.IndexOf('\0')>=0 || new UTF8Encoding(false,true).GetByteCount(script)>1048576)throw new Exception();
 string exe=Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.System),@"WindowsPowerShell\v1.0\powershell.exe");
 response="{\"error\":\"start_failed\"}";
 temp=Path.Combine(Path.GetTempPath(),"wintable-"+Guid.NewGuid().ToString("N"));
 if(!MakeDirectory(temp,IntPtr.Zero)){temp=null;throw new Exception();}
 string file=Path.Combine(temp,"script.ps1");
 using(var f=new FileStream(file,FileMode.CreateNew,FileAccess.Write,FileShare.None)){byte[] bom=new byte[]{239,187,191};f.Write(bom,0,bom.Length);byte[] data=Encoding.UTF8.GetBytes(script);f.Write(data,0,data.Length);}
 string args="-NoProfile -NonInteractive -File "+Q(file);
 response="{\"error\":\"start_failed\"}";using(var child=new Process()){child.StartInfo=new ProcessStartInfo(exe,args){UseShellExecute=false,RedirectStandardOutput=true,RedirectStandardError=true,CreateNoWindow=true};if(!child.Start())throw new Exception();started=true;bool oc=false,ec=false;var o=Task.Factory.StartNew(()=>Drain(child.StandardOutput.BaseStream,out oc));var e=Task.Factory.StartNew(()=>Drain(child.StandardError.BaseStream,out ec));child.WaitForExit();Task.WaitAll(o,e);response="{\"exit_code\":"+child.ExitCode+",\"stdout\":\""+o.Result+"\",\"stderr\":\""+e.Result+"\",\"stdout_truncated\":"+oc.ToString().ToLowerInvariant()+",\"stderr_truncated\":"+ec.ToString().ToLowerInvariant()+"}";}
 }catch{if(started)response="{\"error\":\"execution_failed\"}";}
 finally{if(temp!=null)try{File.Delete(Path.Combine(temp,"script.ps1"));Directory.Delete(temp);}catch{response="{\"error\":\"execution_failed\"}";}}
 byte[] output=Encoding.UTF8.GetBytes(response);Console.OpenStandardOutput().Write(output,0,output.Length);}
}
