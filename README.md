## 简介

​	mongosync工具支持普通同步和增量同步。增量同步模式要求源库必须是副本集，因为增量同步是通过oplog来实现的。单节点的mongodb实例也可以开启oplog。

​	mongosync还支持库名映射、集合名映射。普通同步模式下，可以安全使用。增量同步模式下，对于某些command类型的oplog操作可能无法正确重放。

## 参数介绍

```bash
[root@physerver tmp]# ./mongosync --help
Usage of ./mongosync:
  -db string
        databases to sync.Default for all dbs in instance. Format:<database-name,...>. Namespace control sub-parameters: --nsExclude,--nsInclude,--nsFrom_To
  -dbFrom_To string
        rename matching databasename. Format:<src_dbname:dst_dbname,...>
  -dst_auth_db string
        the destination mongodb server's auth db
  -dst_host string
        the destination mongodb server's ip
  -dst_passwd string
        the destination mongodb server's logging password
  -dst_port int
        the destination mongodb server's port (default 27017)
  -dst_user string
        the destination mongodb server's logging user
  -no_index
        whether to clone the db or collection corresponding index
  -nsExclude string
        exclude matching namespaces. Format:<namespace,...>
  -nsFrom_To string
        rename matching namespaces. Format:<src_namespace:dst_namespace,...>
  -nsInclude string
        include matching namespaces. Format:<namespace,...>
  -op_end string
        the end timestamp to sync oplog,the default value of "0,0" indicates the current latest oplog. Format:<"m,n"> (default "0,0")
  -op_start string
        the start timestamp to sync oplog. Format:<"m,n"> (default "0,0")
  -oplog
        whether to enable oplog for incremental synchronization
  -replayoplog
        repaly oplog,must have matching op_start
  -src_auth_db string
        the source mongodb server's auth db
  -src_host string
        the source mongodb server's ip (default "0.0.0.0")
  -src_op_ns string
        the namespace of the source of oplog. Format:<namespace,...> (default "local.oplog.rs")
  -src_passwd string
        the source mongodb server's logging password
  -src_port int
        the source mongodb server's port (default 27017)
  -src_user string
        the source mongodb server's logging user
  -sync_oplog
        whether to synchronize oplog to the destination mongodb
  -threadNum int
        Number of threads performing collection synchronization (default 20)
```

## 使用示例

1、普通同步，只对GlobalDB和CUST_U_TEST这两个库进行数据同步

```bash
[root@physerver tmp]# ./mongosync --dst_host 192.168.5.245 --dst_port 8088 --dst_user admin --dst_passwd 111111 --dst_auth_db admin --src_host 192.168.5.182 --src_port 8088 -db GlobalDB,CUST_U_TEST
```

2、使用--oplog选项，进行增量实时同步 (推荐，但要求src开启oplog)

```bash
[root@physerver tmp]# ./mongosync --dst_host 192.168.5.245 --dst_port 8088 --dst_user admin --dst_passwd 111111 --dst_auth_db admin --src_host 192.168.5.182 --src_port 8088 -db GlobalDB,CUST_U_TEST --oplog
```

3、省略--db选项，实时同步除了admin、local库之外的所有库

```bash
[root@physerver tmp]# ./mongosync --dst_host 192.168.5.245 --dst_port 8088 --dst_user admin --dst_passwd 111111 --dst_auth_db admin --src_host 192.168.5.182 --src_port 8088 --oplog
```

4、对GlobalDB,CUST_U_TEST库进行实时同步，但是排除CUST_U_TEST.files.file,CUST_U_TEST.files.chunks集合

```bash
[root@physerver tmp]# ./mongosync --dst_host 192.168.5.245 --dst_port 8088 --dst_user admin --dst_passwd 111111 --dst_auth_db admin --src_host 192.168.5.182 --src_port 8088 -db GlobalDB,CUST_U_TEST --nsExclude "CUST_U_TEST.files.file,CUST_U_TEST.files.chunks" --oplog
```

5、只实时同步CUST_U_TEST.files.file,CUST_U_TEST.files.chunks集合

```bash
[root@physerver tmp]# ./mongosync --dst_host 192.168.5.245 --dst_port 8088 --dst_user admin --dst_passwd 111111 --dst_auth_db admin --src_host 192.168.5.182 --src_port 8088 -db CUST_U_TEST --nsInclude "CUST_U_TEST.files.file,CUST_U_TEST.files.chunks" --oplog
```

6、将源库名CUST_U_TEST同步成目标库名为MYTEST

```bash
[root@physerver tmp]# ./mongosync --dst_host 192.168.5.245 --dst_port 8088 --dst_user admin --dst_passwd 111111 --dst_auth_db admin --src_host 192.168.5.182 --src_port 8088 -db CUST_U_TEST --dbFrom_To CUST_U_TEST:MYTEST
```

7、将源集合名CUST_U_TEST.People同步成目标库名为CUST_U_TEST.Persion

```bash
[root@physerver tmp]# ./mongosync --dst_host 192.168.5.245 --dst_port 8088 --dst_user admin --dst_passwd 111111 --dst_auth_db admin --src_host 192.168.5.182 --src_port 8088 -db CUST_U_TEST --nsFrom_To CUST_U_TEST.People:CUST_U_TEST.Persion
```

8、指定同步线程数量为5，默认为20个线程

```bash
[root@physerver tmp]# ./mongosync --dst_host 192.168.5.245 --dst_port 8088 --dst_user admin --dst_passwd 111111 --dst_auth_db admin --src_host 192.168.5.182 --src_port 8088 -db GlobalDB,CUST_U_TEST --threadNum 5
```

9、覆盖那些“_id”字段已经存在的文档

```bash
[root@physerver tmp]# ./mongosync --src_host 192.168.5.245 --src_port 8088 --src_user admin --src_passwd 111111 --src_auth_db admin --dst_host 192.168.5.182 --dst_port 8088 -db GlobalDB --overwrite
```