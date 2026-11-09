public partial class Receiver {
 [DllImport("kernel32",CharSet=CharSet.Unicode,SetLastError=true)]
 static extern SafeFileHandle CreateFile(string p,uint a,uint s,IntPtr sa,uint mode,uint flags,IntPtr tpl);
 [DllImport("kernel32",SetLastError=true)]
 static extern bool GetFileInformationByHandle(SafeFileHandle h,[Out] byte[] b);
 static byte[] Info(SafeFileHandle h,bool dir){var b=new byte[52];if(h.IsInvalid || !GetFileInformationByHandle(h,b) || (BitConverter.ToUInt32(b,0)&0x410)!=(dir?16u:0u) || (!dir && BitConverter.ToUInt32(b,40)!=1))throw new Exception();return b;}
 static SafeFileHandle Open(string p,bool dir){var h=CreateFile(p,dir?0u:0x80000000u,1,IntPtr.Zero,3,0x02200000,IntPtr.Zero);try{Info(h,dir);return h;}catch{h.Dispose();throw;}}
 static byte[] Hash(FileStream f){f.Position=0;using(var h=System.Security.Cryptography.SHA256.Create())return h.ComputeHash(f);}
 static bool Same(byte[] a,byte[] b){if(a.Length!=b.Length)return false;for(int j=0;j<a.Length;j++)if(a[j]!=b[j])return false;return true;}
 static bool Stable(byte[] a,byte[] b){for(int j=0;j<a.Length;j++)if((j<12 || j>=20) && a[j]!=b[j])return false;return true;}
 public static void Run(){var guards=new List<SafeFileHandle>();try{
 byte[] x=Read(Console.OpenStandardInput(),2097153);if(x.Length>2097152)throw new Exception();var p=new Receiver();p.s=new UTF8Encoding(false,true).GetString(x);var d=(Dictionary<string,object>)p.V(0);p.W();if(p.i!=p.s.Length || d.Count!=2 || (string)d["module"]!="fetch")throw new Exception();var a=(Dictionary<string,object>)d["args"];if(a.Count!=1)throw new Exception();string path=(string)a["src"];
 if(!System.Text.RegularExpressions.Regex.IsMatch(path,@"\A[A-Za-z]:\\[^\x00-\x1f\x7f-\x9f\""<>:|?*/]+\z"))throw new Exception();foreach(string c in path.Substring(3).Split('\\')){if(c.Length==0 || c.EndsWith(".") || c.EndsWith(" ") || System.Text.RegularExpressions.Regex.IsMatch(c.Split('.')[0],@"\A(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])\z",System.Text.RegularExpressions.RegexOptions.IgnoreCase))throw new Exception();}
 var dirs=new Stack<string>();for(string v=Path.GetDirectoryName(path);v!=null;v=Path.GetDirectoryName(v))dirs.Push(v);while(dirs.Count>0)guards.Add(Open(dirs.Pop(),true));
 using(var h=Open(path,false))using(var f=new FileStream(h,FileAccess.Read)){
 byte[] info=Info(h,false);long size=f.Length;if(size>1073741824)throw new Exception();byte[] before=Hash(f);f.Position=0;var output=Console.OpenStandardOutput();byte[] header=Encoding.ASCII.GetBytes("WTF1");output.Write(header,0,4);byte[] n=BitConverter.GetBytes(size);output.Write(n,0,8);
 using(var hash=System.Security.Cryptography.SHA256.Create()){byte[] buf=new byte[65536];long total=0;for(;;){int count=f.Read(buf,0,buf.Length);if(count==0)break;total+=count;if(total>size)throw new Exception();hash.TransformBlock(buf,0,count,null,0);output.Write(buf,0,count);}hash.TransformFinalBlock(new byte[0],0,0);if(total!=size || !Same(before,hash.Hash) || !Same(before,Hash(f)) || !Stable(info,Info(h,false)))throw new Exception();output.Write(hash.Hash,0,32);output.Flush();}
 }
 }catch{Environment.ExitCode=1;}finally{foreach(var h in guards)h.Dispose();}}
}
