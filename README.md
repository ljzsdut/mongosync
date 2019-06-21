## 简介

​	mongosync工具支持普通同步和增量同步。增量同步模式(--oplog或--sync_oplog)要求源库必须是副本集，因为增量同步是通过oplog来实现的，而且要求进行同步的源库用户具有访问admin库的权限。全量同步(默认)之要求源库用户对目标库有读取权限即可。说明：单节点的mongodb实例也可以开启oplog。

​	mongosync工具主要有两种操作：数据同步（默认）、重放oplog（--replayoplog）。数据同步操作又分为全量同步、增量同步、增量异步3种方式，它们主要有如下特性：

- 全量同步(默认模式)：基于快照方式的同步，目标库的数据为执行mongosync命令时刻的快照数据。优点：该方式不要求开启oplog，同步的源用户不要求对admin库有访问权限。缺点是不能实时进行数据同步。
- 增量同步(--oplog)：先进行基于快照的全量同步，然后再进行基于oplog的增量同步。优点：可以实现数据的实时同步；缺点：要求开启oplog，而且同步的源用户要求对admin库有访问权限。
- 增量异步(--sync_oplog)：我们知道，用于存储oplog的local.oplog.rs集合是一个固定大小的集合，一旦集合写满，新数据便会从集合的首部开始继续覆盖写入，会导致集合最开始的记录丢失。基于这个情况，如果增量同步时，源库数据量很大，全量同步阶段耗时很长，可能会出现在开始进行增量之前，local.oplog.rs集合首部的一些数据已经被覆盖，此时oplog无法重放成功。“全量异步”模式就是为了解决该情况的一种同步实现，它会在全量同步开始后，将新产生的oplog条目同时记录的目标实例的syncoplog.oplog.rs集合中，待我们完成全量同步后，使用`mongosync --replayoplog --src_op_ns syncoplog.oplog.rs --op_start <m,n>`命令要异步重放oplog。优点：可以实现数据的实时同步；缺点：要求开启oplog，而且同步的源用户要求对admin库有访问权限。

​	此外，mongosync还支持库名映射、集合名映射。普通同步模式下，可以安全使用。增量同步模式下，对于某些command类型的oplog操作可能无法正确重放。

## 参数介绍

```bash
[root@physerver tmp]# ./mongosync --help
Usage of ./mongosync:
  -db string
        databases to sync.Default for all dbs in instance. Format:<database-name,...>. Namespace control sub-parameters: --nsExclude,--nsInclude,--nsFrom_To
  -dbFrom_To string
        rename matching databasename. Format:<src_dbname:dst_dbname,...>
  -dd string
        the destination mongodb server's auth db
  -dh string
        the destination mongodb server's ip
  -dp string
        the destination mongodb server's logging password
  -dP int
        the destination mongodb server's port (default 27017)
  -du string
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
  -sd string
        the source mongodb server's auth db
  -sh string
        the source mongodb server's ip (default "0.0.0.0")
  -src_op_ns string
        the namespace of the source of oplog. Format:<namespace,...> (default "local.oplog.rs")
  -sp string
        the source mongodb server's logging password
  -sP int
        the source mongodb server's port (default 27017)
  -su string
        the source mongodb server's logging user
  -sync_oplog
        whether to synchronize oplog to the destination mongodb
  -threadNum int
        Number of threads performing collection synchronization (default 20)
```

## 使用示例

1、普通同步，只对GlobalDB和CUST_U_TEST这两个库进行数据同步

```bash
[root@physerver tmp]# ./mongosync --dh 192.168.5.245 --dP 8088 --du admin --dp 111111 --dd admin --sh 192.168.5.182 --sP 8088 -db GlobalDB,CUST_U_TEST
```

2、使用--oplog选项，进行增量实时同步 (推荐，但要求src开启oplog)

```bash
[root@physerver tmp]# ./mongosync --dh 192.168.5.245 --dP 8088 --du admin --dp 111111 --dd admin --sh 192.168.5.182 --sP 8088 -db GlobalDB,CUST_U_TEST --oplog
```

3、省略--db选项，实时同步除了admin、local库之外的所有库

```bash
[root@physerver tmp]# ./mongosync --dh 192.168.5.245 --dP 8088 --du admin --dp 111111 --dd admin --sh 192.168.5.182 --sP 8088 --oplog
```

4、对GlobalDB,CUST_U_TEST库进行实时同步，但是排除CUST_U_TEST.files.file,CUST_U_TEST.files.chunks集合

```bash
[root@physerver tmp]# ./mongosync --dh 192.168.5.245 --dP 8088 --du admin --dp 111111 --dd admin --sh 192.168.5.182 --sP 8088 -db GlobalDB,CUST_U_TEST --nsExclude "CUST_U_TEST.files.file,CUST_U_TEST.files.chunks" --oplog
```

5、只实时同步CUST_U_TEST.files.file,CUST_U_TEST.files.chunks集合

```bash
[root@physerver tmp]# ./mongosync --dh 192.168.5.245 --dP 8088 --du admin --dp 111111 --dd admin --sh 192.168.5.182 --sP 8088 -db CUST_U_TEST --nsInclude "CUST_U_TEST.files.file,CUST_U_TEST.files.chunks" --oplog
```

6、将源库名CUST_U_TEST同步成目标库名为MYTEST

```bash
[root@physerver tmp]# ./mongosync --dh 192.168.5.245 --dP 8088 --du admin --dp 111111 --dd admin --sh 192.168.5.182 --sP 8088 -db CUST_U_TEST --dbFrom_To CUST_U_TEST:MYTEST
```

7、将源集合名CUST_U_TEST.People同步成目标库名为CUST_U_TEST.Persion

```bash
[root@physerver tmp]# ./mongosync --dh 192.168.5.245 --dP 8088 --du admin --dp 111111 --dd admin --sh 192.168.5.182 --sP 8088 -db CUST_U_TEST --nsFrom_To CUST_U_TEST.People:CUST_U_TEST.Persion
```

8、指定同步线程数量为5，默认为20个线程

```bash
[root@physerver tmp]# ./mongosync --dh 192.168.5.245 --dP 8088 --du admin --dp 111111 --dd admin --sh 192.168.5.182 --sP 8088 -db GlobalDB,CUST_U_TEST --threadNum 5
```

9、覆盖那些“_id”字段已经存在的文档

```bash
[root@physerver tmp]# ./mongosync --sh 192.168.5.245 --sP 8088 --su admin --sp 111111 --sd admin --dh 192.168.5.182 --dP 8088 -db GlobalDB --overwrite
```