public partial class Receiver {
 [DllImport("kernel32",CharSet=CharSet.Unicode,SetLastError=true)]
 static extern SafeFileHandle CreateFile(string p,uint a,uint s,IntPtr sa,uint mode,uint flags,IntPtr tpl);
 [DllImport("kernel32")]
 static extern bool GetFileInformationByHandle(SafeFileHandle h,[Out] byte[] b);
 static SafeFileHandle Guard(string p){var h=CreateFile(p,0,3,IntPtr.Zero,3,0x02200000,IntPtr.Zero);var b=new byte[52];if(h.IsInvalid || !GetFileInformationByHandle(h,b) || (BitConverter.ToUInt32(b,0)&0x410)!=0x10){h.Dispose();throw new Exception();}return h;}
 static bool Eq(FileStream f,byte[] b){if(f.Length!=b.Length)return false;for(int n=0;n<b.Length;n++)if(f.ReadByte()!=b[n])return false;return true;}
 static FileStream Open(string p){var h=CreateFile(p,0x80000000,5,IntPtr.Zero,3,0x02200000,IntPtr.Zero);if(h.IsInvalid){int e=Marshal.GetLastWin32Error();h.Dispose();if(e==2)return null;throw new Exception();}try{var b=new byte[52];if(!GetFileInformationByHandle(h,b) || (BitConverter.ToUInt32(b,0)&0x410)!=0)throw new Exception();return new FileStream(h,FileAccess.Read);}catch{h.Dispose();throw;}}
 public static void Run(){string r="{\"error\":\"invalid_request\"}";string temp=null;bool commit=false;var g=new List<SafeFileHandle>();try{
 byte[] x=Read(Console.OpenStandardInput(),2097153);if(x.Length>2097152)throw new Exception();var p=new Receiver();p.s=new UTF8Encoding(false,true).GetString(x);var d=(Dictionary<string,object>)p.V(0);p.W();if(p.i!=p.s.Length || d.Count!=2 || (string)d["module"]!="file_content")throw new Exception();var a=(Dictionary<string,object>)d["args"];if(a.Count!=2)throw new Exception();string path=(string)a["path"];byte[] b=new UTF8Encoding(false,true).GetBytes((string)a["content"]);
 if(b.Length>1048576 || !System.Text.RegularExpressions.Regex.IsMatch(path,@"\A[A-Za-z]:\\[^\x00-\x1f\x7f-\x9f\""<>:|?*/]+\z"))throw new Exception();
 foreach(string c in path.Substring(3).Split('\\')){string name=c.Split('.')[0];if(c.Length==0 || c.EndsWith(".") || c.EndsWith(" ") || System.Text.RegularExpressions.Regex.IsMatch(name,@"\A(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])\z",System.Text.RegularExpressions.RegexOptions.IgnoreCase))throw new Exception();}
 r="{\"error\":\"write_failed\"}";string dir=Path.GetDirectoryName(path);var dirs=new Stack<string>();for(string v=dir;v!=null;v=Path.GetDirectoryName(v))dirs.Push(v);while(dirs.Count>0)g.Add(Guard(dirs.Pop()));
 using(var old=Open(path)){bool exists=old!=null;
 if(old!=null && Eq(old,b)){r="{\"changed\":false}";}else{
 string q=Path.Combine(dir,".wintable-"+Guid.NewGuid().ToString("N")+".tmp");using(var f=new FileStream(q,FileMode.CreateNew,FileAccess.Write,FileShare.None)){temp=q;f.Write(b,0,b.Length);f.Flush(true);}
 using(var current=Open(path))if((current!=null)!=exists)throw new Exception();commit=true;
 if(exists)File.Replace(temp,path,null,false);else File.Move(temp,path);
 temp=null;r="{\"changed\":true}";
 }}
 }catch{if(commit)r="{\"error\":\"execution_failed\"}";}finally{if(temp!=null)try{File.Delete(temp);}catch{r="{\"error\":\"execution_failed\"}";}foreach(var h in g)h.Dispose();}
 byte[] output=Encoding.UTF8.GetBytes(r);Console.OpenStandardOutput().Write(output,0,output.Length);
 }
}
